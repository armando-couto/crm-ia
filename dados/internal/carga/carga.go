package carga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/dados/internal/banco"
	"github.com/armando-couto/crm-ia/dados/internal/config"
)

// Fontes aceitas por Executar e por POST /v1/cargas.
const (
	FonteCNPJ = "cnpj"
	FontePGFN = "pgfn"
	FonteTudo = "tudo"
)

var (
	// ErrEmCurso: já há uma carga rodando (neste processo ou em outro).
	ErrEmCurso = errors.New("já há uma carga em curso")
	// ErrAmostra: o serviço em modo amostra nunca acessa a rede.
	ErrAmostra = errors.New("serviço em modo amostra")
	// ErrAguardando: a referência nova do CNPJ ainda está sendo publicada.
	ErrAguardando = errors.New("aguardando publicação completa")
)

// chaveTrava é a trava consultiva que garante uma carga por vez no banco.
const chaveTrava int64 = 0x6372_6d69_6164 // "crmiad"

const gb = 1 << 30

// Servico executa cargas: uma por vez, retomável, sem derrubar o processo
// quando falha.
type Servico struct {
	cfg     config.Config
	Cliente *Cliente
	// MinimoTrabalho é o espaço livre exigido no diretório de trabalho antes
	// de baixar o CNPJ; MinimoTrabalhoPGFN, quando só a dívida é baixada.
	MinimoTrabalho, MinimoTrabalhoPGFN uint64
	// TrabalhadoresCarga é quantos arquivos entram no banco ao mesmo tempo.
	TrabalhadoresCarga int

	mu      sync.Mutex
	rodando bool
}

// Novo monta o serviço de carga.
func Novo(cfg config.Config) *Servico {
	return &Servico{cfg: cfg, Cliente: NovoCliente(cfg.EspacoEntreRequisicoes),
		MinimoTrabalho: 10 * gb, MinimoTrabalhoPGFN: 3 * gb, TrabalhadoresCarga: 4}
}

// Rodando diz se há carga em curso neste processo.
func (s *Servico) Rodando() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rodando
}

// Reservar marca a carga como em curso; devolve false se já havia uma. Quem
// reserva chama ExecutarReservada (a API responde 202 antes de a carga
// começar e 409 quando já há outra).
func (s *Servico) Reservar() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rodando {
		return false
	}
	s.rodando = true
	return true
}

// Executar roda uma carga da fonte e só volta quando ela termina.
func (s *Servico) Executar(ctx context.Context, fonte string) error {
	if s.cfg.Amostra {
		return ErrAmostra
	}
	if !s.Reservar() {
		return ErrEmCurso
	}
	return s.ExecutarReservada(ctx, fonte)
}

// ExecutarReservada roda a carga depois de um Reservar bem-sucedido.
func (s *Servico) ExecutarReservada(ctx context.Context, fonte string) (err error) {
	defer func() {
		s.mu.Lock()
		s.rodando = false
		s.mu.Unlock()
	}()
	if s.cfg.Amostra {
		return ErrAmostra
	}
	if fonte != FonteCNPJ && fonte != FontePGFN && fonte != FonteTudo {
		return fmt.Errorf("fonte desconhecida: %q", fonte)
	}
	// pool próprio da carga: sem statement_timeout e separado da busca
	pool, err := banco.Conectar(ctx, s.cfg.DatabaseURL, int32(s.TrabalhadoresCarga+4), map[string]string{"statement_timeout": "0", "application_name": "crmia-dados-carga"})
	if err != nil {
		return err
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var travou bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, chaveTrava).Scan(&travou); err != nil {
		return err
	}
	if !travou {
		return ErrEmCurso
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, chaveTrava)
	}()

	e := &execucao{s: s, pool: pool, conn: conn, fonte: fonte, inicio: time.Now()}
	semCancelar := context.WithoutCancel(ctx)
	// execução anterior que ficou aberta (queda ou SIGTERM): esta a substitui
	if _, err := pool.Exec(ctx, `UPDATE cargas SET finalizada_em = now(), estado = 'erro', erro = 'interrompida; retomada pela execução seguinte' WHERE finalizada_em IS NULL`); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cargas (fonte, estado, etapa) VALUES ($1, 'baixando', 'conferindo as fontes') RETURNING id`, fonte).Scan(&e.id); err != nil {
		return err
	}
	slog.Info("carga iniciada", "fonte", fonte, "id", e.id)

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pânico na carga: %v", r)
		}
		est, _ := json.Marshal(e.estatisticas())
		switch {
		case err == nil:
			_, _ = pool.Exec(semCancelar, `UPDATE cargas SET estado = 'ociosa', etapa = $2, percentual = 100, erro = '', estatisticas = $3, finalizada_em = now(), atualizada_em = now() WHERE id = $1`, e.id, e.resumo, est)
			slog.Info("carga concluída", "id", e.id, "resultado", e.resumo, "duracao", time.Since(e.inicio).Round(time.Second).String())
		case errors.Is(err, ErrAguardando):
			_, _ = pool.Exec(semCancelar, `UPDATE cargas SET estado = 'ociosa', etapa = 'aguardando publicação completa', percentual = 0, erro = '', estatisticas = $2, finalizada_em = now(), atualizada_em = now() WHERE id = $1`, e.id, est)
			slog.Info("carga adiada: referência nova ainda em publicação", "id", e.id, "referencia", e.refCNPJ)
		case ctx.Err() != nil:
			// encerramento limpo (SIGTERM): a linha fica aberta e a próxima
			// subida retoma, mesmo com a carga automática desligada
			_, _ = pool.Exec(semCancelar, `UPDATE cargas SET etapa = 'interrompida; retoma na próxima subida', atualizada_em = now() WHERE id = $1`, e.id)
			slog.Warn("carga interrompida pelo encerramento do serviço", "id", e.id)
			err = ctx.Err()
		default:
			_, _ = pool.Exec(semCancelar, `UPDATE cargas SET estado = 'erro', erro = $2, estatisticas = $3, finalizada_em = now(), atualizada_em = now() WHERE id = $1`, e.id, err.Error(), est)
			slog.Error("carga falhou; a base anterior continua em uso", "id", e.id, "erro", err)
		}
	}()
	return e.rodar(ctx)
}

// Pendente devolve a fonte de uma carga interrompida (linha aberta), para a
// subida do serviço retomá-la.
func Pendente(ctx context.Context, pool *pgxpool.Pool) (string, bool) {
	var fonte string
	err := pool.QueryRow(ctx, `SELECT fonte FROM cargas WHERE finalizada_em IS NULL ORDER BY id DESC LIMIT 1`).Scan(&fonte)
	return fonte, err == nil
}

// ---------------------------------------------------------------- execução

type execucao struct {
	s      *Servico
	pool   *pgxpool.Pool
	conn   *pgxpool.Conn // conexão dedicada: trava, construção e troca
	fonte  string
	id     int64
	inicio time.Time
	resumo string

	refCNPJ, refPGFN string

	mu        sync.Mutex
	est       map[string]Estatisticas // por tabela de carga
	ultimaAtu time.Time
	bytes     map[string]int64
	totalB    int64
}

func (e *execucao) estatisticas() map[string]Estatisticas {
	e.mu.Lock()
	defer e.mu.Unlock()
	copia := make(map[string]Estatisticas, len(e.est))
	for k, v := range e.est {
		copia[k] = v
	}
	return copia
}

func (e *execucao) somar(tabela string, est Estatisticas) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.est == nil {
		e.est = map[string]Estatisticas{}
	}
	t := e.est[tabela]
	t.Somar(est)
	e.est[tabela] = t
}

func (e *execucao) situacao(ctx context.Context, estado, etapa string, percentual int) {
	ref := strings.TrimSpace(strings.Join(naoVazios(e.refCNPJ, e.refPGFN), " + "))
	if _, err := e.pool.Exec(ctx, `UPDATE cargas SET estado = $2, etapa = $3, percentual = $4, referencia = $5, atualizada_em = now() WHERE id = $1`, e.id, estado, etapa, percentual, ref); err != nil && ctx.Err() == nil {
		slog.Warn("não foi possível gravar a situação da carga", "erro", err)
	}
}

func naoVazios(v ...string) []string {
	var s []string
	for _, x := range v {
		if x != "" {
			s = append(s, x)
		}
	}
	return s
}

type metaAtual struct {
	existe           bool
	refCNPJ, refPGFN string
	empresas         int64
	ufs              []string
}

func lerMeta(ctx context.Context, db Executor, esquema string) (metaAtual, error) {
	var m metaAtual
	existe, err := tabelaExiste(ctx, db, esquema, "meta")
	if err != nil || !existe {
		return m, err
	}
	err = db.QueryRow(ctx, fmt.Sprintf(`SELECT referencia_cnpj, referencia_pgfn, empresas, ufs FROM %s.meta LIMIT 1`, esquema)).Scan(&m.refCNPJ, &m.refPGFN, &m.empresas, &m.ufs)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, nil
	}
	m.existe = err == nil
	return m, err
}

func tabelaExiste(ctx context.Context, db Executor, esquema, tabela string) (bool, error) {
	var existe bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = $1 AND tablename = $2)`, esquema, tabela).Scan(&existe)
	return existe, err
}

func (e *execucao) referenciaPGFNCarregada(ctx context.Context) (string, error) {
	existe, err := tabelaExiste(ctx, e.pool, EsquemaPGFN, "meta")
	if err != nil || !existe {
		return "", err
	}
	var ref string
	err = e.pool.QueryRow(ctx, `SELECT referencia FROM pgfn.meta LIMIT 1`).Scan(&ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return ref, err
}

func (e *execucao) rodar(ctx context.Context) error {
	cfg := e.s.cfg
	if err := os.MkdirAll(cfg.DirTrabalho, 0o755); err != nil {
		return fmt.Errorf("diretório de trabalho %s: %w", cfg.DirTrabalho, err)
	}
	// construção interrompida: recomeça do passo 5
	if err := executar(ctx, e.conn, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s_novo CASCADE`, EsquemaServico)); err != nil {
		return err
	}
	atual, err := lerMeta(ctx, e.pool, EsquemaServico)
	if err != nil {
		return err
	}
	pgfnCarregada, err := e.referenciaPGFNCarregada(ctx)
	if err != nil {
		return err
	}

	// 1. descobrir a referência mais nova de cada fonte
	var arquivosPGFN, arquivosCNPJ []ArquivoRemoto
	aguardando := false
	if e.fonte != FonteCNPJ {
		ref, arquivos, err := e.descobrirPGFN(ctx)
		if err != nil {
			return err
		}
		if ref != pgfnCarregada {
			e.refPGFN, arquivosPGFN = ref, arquivos
		}
	}
	if e.fonte != FontePGFN {
		ref, arquivos, err := e.descobrirCNPJ(ctx)
		if err != nil {
			return err
		}
		mudouRecorte := atual.existe && strings.Join(atual.ufs, ",") != strings.Join(cfg.UFs, ",")
		if ref != atual.refCNPJ || mudouRecorte {
			elegivel, err := e.publicacaoCompleta(ctx, ref, arquivos)
			if err != nil {
				return err
			}
			if elegivel {
				e.refCNPJ, arquivosCNPJ = ref, arquivos
			} else {
				aguardando = true
			}
		}
	}
	if arquivosPGFN == nil && arquivosCNPJ == nil {
		// nada novo para baixar; ainda pode faltar pôr em uso uma dívida já carregada
		if atual.existe && pgfnCarregada != "" && pgfnCarregada != atual.refPGFN {
			if err := e.construirETrocar(ctx, atual, "", pgfnCarregada); err != nil {
				return err
			}
			e.resumo = "dívida ativa " + pgfnCarregada + " posta em uso"
			return nil
		}
		if aguardando {
			return ErrAguardando
		}
		e.resumo = "nada a fazer: as referências publicadas já estão carregadas"
		return nil
	}

	// 2 a 4. dívida ativa: baixar, ler e agregar por CNPJ
	if arquivosPGFN != nil {
		if err := e.conferirDisco(false); err != nil {
			return err
		}
		if err := e.carregarPGFN(ctx, arquivosPGFN); err != nil {
			return err
		}
		pgfnCarregada = e.refPGFN
	}

	// 2 a 4. CNPJ: baixar, ler em fluxo e conferir
	esquemaCarga := ""
	if arquivosCNPJ != nil {
		if err := e.carregarCNPJ(ctx, arquivosCNPJ); err != nil {
			return err
		}
		esquemaCarga = EsquemaCarga
		if err := e.guardas(ctx, atual); err != nil {
			return err
		}
	} else if !atual.existe {
		if aguardando {
			return ErrAguardando
		}
		e.resumo = "dívida ativa " + pgfnCarregada + " carregada; a base de empresas ainda não existe"
		return nil
	}

	// 5 e 6. construir ao lado, trocar, aquecer e limpar
	if err := e.construirETrocar(ctx, atual, esquemaCarga, pgfnCarregada); err != nil {
		return err
	}
	if esquemaCarga != "" {
		e.limpar(ctx, FonteCNPJ, e.refCNPJ)
		if _, err := e.conn.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, EsquemaCarga)); err != nil {
			slog.Warn("não foi possível apagar as tabelas de carga", "erro", err)
		}
	}
	e.resumo = "base atualizada: " + strings.Join(naoVazios(e.refCNPJ, e.refPGFN), " + ")
	if aguardando {
		return ErrAguardando
	}
	return nil
}

func (e *execucao) descobrirPGFN(ctx context.Context) (string, []ArquivoRemoto, error) {
	refs, err := e.s.Cliente.ReferenciasPGFN(ctx, e.s.cfg.PGFNURL)
	if err != nil {
		return "", nil, fmt.Errorf("listagem da PGFN: %w", err)
	}
	var ultimo error
	for i, ref := range refs {
		if i == 2 {
			break
		}
		arquivos, err := e.s.Cliente.ArquivosPGFN(ctx, e.s.cfg.PGFNURL, ref)
		if err == nil {
			return ref, arquivos, nil
		}
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		ultimo = err // trimestre ainda sem os três zips: tenta o anterior
		slog.Warn("trimestre da PGFN incompleto", "referencia", ref, "erro", err)
	}
	if ultimo == nil {
		ultimo = errors.New("nenhum trimestre publicado")
	}
	return "", nil, fmt.Errorf("PGFN sem trimestre elegível: %w", ultimo)
}

func (e *execucao) descobrirCNPJ(ctx context.Context) (string, []ArquivoRemoto, error) {
	refs, err := e.s.Cliente.ReferenciasCNPJ(ctx, e.s.cfg.CNPJURL)
	if err != nil {
		return "", nil, fmt.Errorf("listagem do CNPJ: %w", err)
	}
	for i, ref := range refs {
		if i == 3 {
			break
		}
		arquivos, err := e.s.Cliente.ArquivosCNPJ(ctx, e.s.cfg.CNPJURL, ref)
		if err != nil {
			return "", nil, fmt.Errorf("listagem do CNPJ %s: %w", ref, err)
		}
		if faltam := FaltamCNPJ(arquivos); len(faltam) > 0 {
			slog.Info("referência do CNPJ ainda incompleta", "referencia", ref, "faltam", len(faltam))
			continue
		}
		return ref, arquivos, nil
	}
	return "", nil, errors.New("CNPJ sem referência completa (37 arquivos) entre as três pastas mais novas")
}

// publicacaoCompleta aplica a regra das duas listagens iguais com intervalo:
// a Receita sobe os 37 arquivos ao longo de horas.
func (e *execucao) publicacaoCompleta(ctx context.Context, ref string, arquivos []ArquivoRemoto) (bool, error) {
	var jaComecou bool
	if err := e.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM carga_arquivos WHERE fonte = 'cnpj' AND referencia = $1)`, ref).Scan(&jaComecou); err != nil {
		return false, err
	}
	if jaComecou {
		return true, nil // já foi considerada completa numa execução anterior
	}
	assinatura := Assinatura(arquivos)
	var anterior string
	var vistoEm time.Time
	err := e.pool.QueryRow(ctx, `SELECT assinatura, visto_em FROM publicacoes WHERE fonte = 'cnpj' AND referencia = $1`, ref).Scan(&anterior, &vistoEm)
	if err == nil && anterior == assinatura && time.Since(vistoEm) >= e.s.cfg.EsperaPublicacao {
		return true, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if err != nil || anterior != assinatura {
		if _, err := e.pool.Exec(ctx, `INSERT INTO publicacoes (fonte, referencia, assinatura) VALUES ('cnpj', $1, $2)
ON CONFLICT (fonte, referencia) DO UPDATE SET assinatura = EXCLUDED.assinatura, visto_em = now()`, ref, assinatura); err != nil {
			return false, err
		}
	}
	e.refCNPJ = ref
	e.situacao(ctx, "baixando", "aguardando publicação completa", 0)
	e.refCNPJ = ""
	return false, nil
}

func (e *execucao) conferirDisco(cnpj bool) error {
	minimo := e.s.MinimoTrabalhoPGFN
	if cnpj {
		minimo = e.s.MinimoTrabalho
	}
	livre, err := espacoLivre(e.s.cfg.DirTrabalho)
	if err != nil {
		return fmt.Errorf("não foi possível medir o espaço livre em %s: %w", e.s.cfg.DirTrabalho, err)
	}
	if livre < minimo {
		return fmt.Errorf("espaço insuficiente em %s: %.1f GB livres, a carga exige %.1f GB; nada foi baixado", e.s.cfg.DirTrabalho, float64(livre)/gb, float64(minimo)/gb)
	}
	if cnpj {
		// o serviço não enxerga o disco do PostgreSQL: vale o que o operador declara
		precisa := 70
		if len(e.s.cfg.UFs) > 0 {
			precisa = 20
		}
		if e.s.cfg.DiscoBancoGB < precisa {
			return fmt.Errorf("espaço insuficiente no banco: a carga exige %d GB livres no disco do PostgreSQL e DADOS_DISCO_MINIMO_GB declara %d; libere espaço (ou use DADOS_UFS) e ajuste a variável; nada foi baixado", precisa, e.s.cfg.DiscoBancoGB)
		}
	}
	return nil
}

// ---------------------------------------------------------------- arquivos

type arquivo struct {
	ArquivoRemoto
	estado string
	linhas int64
}

func (e *execucao) caminho(fonte, ref, nome string) string {
	return filepath.Join(e.s.cfg.DirTrabalho, fonte+"-"+ref, nome)
}

// registrar grava os arquivos da referência na tabela de controle e devolve
// o estado de cada um. Arquivo cujo ETag ou tamanho mudou recomeça do zero.
func (e *execucao) registrar(ctx context.Context, fonte, ref string, remotos []ArquivoRemoto) ([]*arquivo, error) {
	if err := os.MkdirAll(filepath.Dir(e.caminho(fonte, ref, "x")), 0o755); err != nil {
		return nil, err
	}
	// referências antigas da mesma fonte: zips e controle saem
	linhas, err := e.pool.Query(ctx, `DELETE FROM carga_arquivos WHERE fonte = $1 AND referencia <> $2 RETURNING referencia`, fonte, ref)
	if err != nil {
		return nil, err
	}
	antigas, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, a := range antigas {
		_ = os.RemoveAll(filepath.Dir(e.caminho(fonte, a, "x")))
	}
	var arquivos []*arquivo
	for _, r := range remotos {
		a := &arquivo{ArquivoRemoto: r}
		var etag string
		var total int64
		err := e.pool.QueryRow(ctx, `SELECT estado, linhas, etag, total_bytes FROM carga_arquivos WHERE fonte = $1 AND referencia = $2 AND nome = $3`, fonte, ref, r.Nome).Scan(&a.estado, &a.linhas, &etag, &total)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		mudou := err == nil && (etag != r.ETag || total != r.Tamanho)
		if err != nil || mudou || a.estado == "erro" || a.estado == "baixando" {
			if mudou {
				_ = os.Remove(e.caminho(fonte, ref, r.Nome))
				_ = os.Remove(e.caminho(fonte, ref, r.Nome) + ".part")
			}
			a.estado, a.linhas = "pendente", 0
		}
		if a.estado == "baixado" && tamanhoDe(e.caminho(fonte, ref, r.Nome)) != r.Tamanho && r.Tamanho > 0 {
			a.estado = "pendente"
		}
		if _, err := e.pool.Exec(ctx, `INSERT INTO carga_arquivos (fonte, referencia, nome, url, estado, total_bytes, etag, modificado, carga_id, linhas, bytes)
VALUES ($1, $2, $3, $4, $5::text, $6::bigint, $7, $8, $9, $10, CASE WHEN $5::text = 'pendente' THEN 0 ELSE $6::bigint END)
ON CONFLICT (fonte, referencia, nome) DO UPDATE SET url = EXCLUDED.url, estado = EXCLUDED.estado, total_bytes = EXCLUDED.total_bytes,
  etag = EXCLUDED.etag, modificado = EXCLUDED.modificado, carga_id = EXCLUDED.carga_id, linhas = EXCLUDED.linhas, bytes = EXCLUDED.bytes, erro = '', atualizado_em = now()`,
			fonte, ref, r.Nome, r.URL, a.estado, r.Tamanho, r.ETag, r.Modificado, e.id, a.linhas); err != nil {
			return nil, err
		}
		arquivos = append(arquivos, a)
	}
	return arquivos, nil
}

func (e *execucao) marcar(ctx context.Context, fonte, ref string, a *arquivo, estado string, bytes, linhas, lidas int64, falha string) {
	a.estado = estado
	if _, err := e.pool.Exec(context.WithoutCancel(ctx), `UPDATE carga_arquivos SET estado = $4, bytes = $5, linhas = $6, linhas_lidas = $7, erro = $8, atualizado_em = now()
WHERE fonte = $1 AND referencia = $2 AND nome = $3`, fonte, ref, a.Nome, estado, bytes, linhas, lidas, falha); err != nil {
		slog.Warn("não foi possível gravar o estado do arquivo", "arquivo", a.Nome, "erro", err)
	}
}

// emParalelo roda f para cada arquivo com n trabalhadores e para no primeiro
// erro.
func emParalelo(ctx context.Context, n int, arquivos []*arquivo, f func(ctx context.Context, a *arquivo) error) error {
	ctx, cancelar := context.WithCancel(ctx)
	defer cancelar()
	fila := make(chan *arquivo)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var primeiro error
	for i := 0; i < max(1, n); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for a := range fila {
				if ctx.Err() != nil {
					continue
				}
				if err := f(ctx, a); err != nil {
					mu.Lock()
					if primeiro == nil {
						primeiro = err
					}
					mu.Unlock()
					cancelar()
				}
			}
		}()
	}
	for _, a := range arquivos {
		fila <- a
	}
	close(fila)
	wg.Wait()
	return primeiro
}

// baixar traz os arquivos pendentes para o diretório de trabalho, com no
// máximo DADOS_CONEXOES transferências ao mesmo tempo.
func (e *execucao) baixar(ctx context.Context, fonte, ref string, arquivos []*arquivo, de, ate int) error {
	var total int64
	e.mu.Lock()
	e.bytes = map[string]int64{}
	for _, a := range arquivos {
		total += a.Tamanho
		if a.estado != "pendente" {
			e.bytes[a.Nome] = a.Tamanho
		}
	}
	e.totalB = total
	e.mu.Unlock()
	var pendentes []*arquivo
	for _, a := range arquivos {
		if a.estado == "pendente" {
			pendentes = append(pendentes, a)
		}
	}
	// os pequenos primeiro: catálogos chegam logo e o erro de leiaute aparece cedo
	sort.SliceStable(pendentes, func(i, j int) bool { return pendentes[i].Tamanho < pendentes[j].Tamanho })
	e.situacao(ctx, "baixando", fmt.Sprintf("baixando %s %s", fonte, ref), de)
	return emParalelo(ctx, e.s.cfg.Conexoes, pendentes, func(ctx context.Context, a *arquivo) error {
		e.marcar(ctx, fonte, ref, a, "baixando", tamanhoDe(e.caminho(fonte, ref, a.Nome)+".part"), 0, 0, "")
		var ultimaGravacao time.Time
		err := e.s.Cliente.Baixar(ctx, a.ArquivoRemoto, e.caminho(fonte, ref, a.Nome), func(b int64) {
			e.mu.Lock()
			e.bytes[a.Nome] = b
			var soma int64
			for _, v := range e.bytes {
				soma += v
			}
			gravarGeral := time.Since(e.ultimaAtu) > 3*time.Second
			if gravarGeral {
				e.ultimaAtu = time.Now()
			}
			e.mu.Unlock()
			if time.Since(ultimaGravacao) > 3*time.Second {
				ultimaGravacao = time.Now()
				e.marcar(ctx, fonte, ref, a, "baixando", b, 0, 0, "")
			}
			if gravarGeral && total > 0 {
				e.situacao(ctx, "baixando", fmt.Sprintf("baixando %s %s", fonte, ref), de+int(float64(ate-de)*float64(soma)/float64(total)))
			}
		})
		if err != nil {
			if ctx.Err() == nil {
				e.marcar(ctx, fonte, ref, a, "erro", tamanhoDe(e.caminho(fonte, ref, a.Nome)+".part"), 0, 0, err.Error())
			}
			return err
		}
		e.marcar(ctx, fonte, ref, a, "baixado", tamanhoDe(e.caminho(fonte, ref, a.Nome)), 0, 0, "")
		return nil
	})
}

// conferirCarregados aplica a retomada: tabelas UNLOGGED são esvaziadas pelo
// PostgreSQL depois de um desligamento sujo, então cada arquivo "carregado"
// tem de ter na tabela de carga as linhas que o controle diz. Senão volta a
// "baixado" e é relido do zip (por isso os zips só somem depois da troca).
func (e *execucao) conferirCarregados(ctx context.Context, fonte, ref string, arquivos []*arquivo, contagem func(a *arquivo) (int64, error)) error {
	for _, a := range arquivos {
		if a.estado != "carregado" {
			continue
		}
		n, err := contagem(a)
		if err != nil {
			return err
		}
		if n == a.linhas {
			continue
		}
		slog.Warn("tabela de carga não confere com o controle; arquivo será recarregado", "arquivo", a.Nome, "controle", a.linhas, "tabela", n)
		estado := "baixado"
		if a.Tamanho > 0 && tamanhoDe(e.caminho(fonte, ref, a.Nome)) != a.Tamanho {
			estado = "pendente"
		}
		e.marcar(ctx, fonte, ref, a, estado, 0, 0, 0, "")
	}
	return nil
}

func (e *execucao) contagensPorArquivo(ctx context.Context, tabela, expressao string) (map[string]int64, error) {
	linhas, err := e.pool.Query(ctx, fmt.Sprintf(`SELECT %s, count(*) FROM %s.%s GROUP BY 1`, expressao, EsquemaCarga, tabela))
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	m := map[string]int64{}
	for linhas.Next() {
		var arq *string
		var n int64
		if err := linhas.Scan(&arq, &n); err != nil {
			return nil, err
		}
		if arq != nil {
			m[*arq] = n
		}
	}
	return m, linhas.Err()
}

func (e *execucao) carregarCNPJ(ctx context.Context, remotos []ArquivoRemoto) error {
	const fonte = FonteCNPJ
	ref := e.refCNPJ
	// Paises e Motivos fazem parte da publicação, mas a base não os usa
	var usados []ArquivoRemoto
	for _, r := range remotos {
		if _, ok := LeiauteDoArquivo(r.Nome); ok {
			usados = append(usados, r)
		}
	}
	arquivos, err := e.registrar(ctx, fonte, ref, usados)
	if err != nil {
		return err
	}
	if err := CriarTabelasDeCarga(ctx, e.pool, EsquemaCarga); err != nil {
		return err
	}
	contagens := map[string]map[string]int64{}
	if err := e.conferirCarregados(ctx, fonte, ref, arquivos, func(a *arquivo) (int64, error) {
		l, _ := LeiauteDoArquivo(a.Nome)
		if contagens[l.Tabela] == nil {
			c, err := e.contagensPorArquivo(ctx, l.Tabela, "arq")
			if err != nil {
				return 0, err
			}
			contagens[l.Tabela] = c
		}
		return contagens[l.Tabela][strings.TrimSuffix(a.Nome, ".zip")], nil
	}); err != nil {
		return err
	}
	falta := false
	for _, a := range arquivos {
		falta = falta || a.estado == "pendente"
	}
	if falta {
		if err := e.conferirDisco(true); err != nil {
			return err
		}
	}
	if err := e.baixar(ctx, fonte, ref, arquivos, 5, 45); err != nil {
		return err
	}

	var feitos int
	var mu sync.Mutex
	e.situacao(ctx, "carregando", "lendo os arquivos do CNPJ "+ref, 45)
	return emParalelo(ctx, e.s.TrabalhadoresCarga, arquivos, func(ctx context.Context, a *arquivo) error {
		if a.estado != "carregado" {
			l, _ := LeiauteDoArquivo(a.Nome)
			var ultima time.Time
			est, err := CarregarZip(ctx, e.pool, EsquemaCarga, l, e.caminho(fonte, ref, a.Nome), e.s.cfg.UFs, func(lidas int64) {
				if time.Since(ultima) > 3*time.Second {
					ultima = time.Now()
					e.marcar(ctx, fonte, ref, a, "baixado", a.Tamanho, 0, lidas, "")
				}
			})
			if err != nil {
				if ctx.Err() == nil {
					e.marcar(ctx, fonte, ref, a, "erro", a.Tamanho, 0, est.Lidas, err.Error())
				}
				return fmt.Errorf("carga do CNPJ %s abortada, a base anterior fica em uso: %w", ref, err)
			}
			a.linhas = est.Gravadas
			e.marcar(ctx, fonte, ref, a, "carregado", a.Tamanho, est.Gravadas, est.Lidas, "")
			e.somar(l.Tabela, est)
			slog.Info("arquivo carregado", "arquivo", a.Nome, "lidas", est.Lidas, "gravadas", est.Gravadas, "nuls", est.NULs, "cortes_cpf", est.CortesCPF.Total())
		}
		mu.Lock()
		feitos++
		p := 45 + 30*feitos/len(arquivos)
		mu.Unlock()
		e.situacao(ctx, "carregando", "lendo os arquivos do CNPJ "+ref, p)
		return nil
	})
}

// guardas: linhas fantasma de Empresas, cnpj_basico único e contagem contra
// a carga anterior.
func (e *execucao) guardas(ctx context.Context, atual metaAtual) error {
	e.situacao(ctx, "carregando", "conferindo a carga", 75)
	if _, err := e.pool.Exec(ctx, `DELETE FROM carga.empresas WHERE razao_social IS NULL AND (natureza IS NULL OR natureza = '0000')`); err != nil {
		return err
	}
	var repetidos, cnpjsRepetidos, ativos int64
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM carga.empresas GROUP BY cnpj_basico HAVING count(*) > 1 LIMIT 10) r`).Scan(&repetidos); err != nil {
		return err
	}
	if repetidos > 0 {
		return fmt.Errorf("guarda da carga: há cnpj_basico repetido em Empresas (%d ou mais); a base anterior fica em uso", repetidos)
	}
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM carga.estabelecimentos GROUP BY cnpj HAVING count(*) > 1 LIMIT 10) r`).Scan(&cnpjsRepetidos); err != nil {
		return err
	}
	if cnpjsRepetidos > 0 {
		return fmt.Errorf("guarda da carga: há CNPJ repetido em Estabelecimentos (%d ou mais); a base anterior fica em uso", cnpjsRepetidos)
	}
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM carga.estabelecimentos`).Scan(&ativos); err != nil {
		return err
	}
	if ativos == 0 {
		return errors.New("guarda da carga: nenhum estabelecimento ativo foi carregado; a base anterior fica em uso")
	}
	mesmoRecorte := strings.Join(atual.ufs, ",") == strings.Join(e.s.cfg.UFs, ",")
	if atual.existe && mesmoRecorte && atual.empresas > 0 && ativos*10 < atual.empresas*8 {
		return fmt.Errorf("guarda da carga: %d estabelecimentos ativos contra %d da base em uso (queda maior que 20%%); a base anterior fica em uso", ativos, atual.empresas)
	}
	if _, err := e.pool.Exec(ctx, `ANALYZE carga.estabelecimentos; ANALYZE carga.empresas; ANALYZE carga.simples; ANALYZE carga.socios`); err != nil {
		return err
	}
	return nil
}

func (e *execucao) carregarPGFN(ctx context.Context, remotos []ArquivoRemoto) error {
	const fonte = FontePGFN
	ref := e.refPGFN
	arquivos, err := e.registrar(ctx, fonte, ref, remotos)
	if err != nil {
		return err
	}
	if err := CriarTabelasDeCarga(ctx, e.pool, EsquemaCarga); err != nil {
		return err
	}
	var contagens map[string]int64
	if err := e.conferirCarregados(ctx, fonte, ref, arquivos, func(a *arquivo) (int64, error) {
		if contagens == nil {
			c, err := e.contagensPorArquivo(ctx, "pgfn", "split_part(arq, '/', 1)")
			if err != nil {
				return 0, err
			}
			contagens = c
		}
		return contagens[strings.TrimSuffix(a.Nome, ".zip")], nil
	}); err != nil {
		return err
	}
	if err := e.baixar(ctx, fonte, ref, arquivos, 1, 3); err != nil {
		return err
	}
	e.situacao(ctx, "carregando", "lendo a dívida ativa "+ref, 3)
	if err := emParalelo(ctx, min(3, e.s.TrabalhadoresCarga), arquivos, func(ctx context.Context, a *arquivo) error {
		if a.estado == "carregado" {
			return nil
		}
		est, err := CarregarZipPGFN(ctx, e.pool, EsquemaCarga, e.caminho(fonte, ref, a.Nome))
		if err != nil {
			if ctx.Err() == nil {
				e.marcar(ctx, fonte, ref, a, "erro", a.Tamanho, 0, est.Lidas, err.Error())
			}
			return fmt.Errorf("carga da PGFN %s abortada, a base anterior fica em uso: %w", ref, err)
		}
		a.linhas = est.Gravadas
		e.marcar(ctx, fonte, ref, a, "carregado", a.Tamanho, est.Gravadas, est.Lidas, "")
		e.somar("pgfn", est)
		slog.Info("arquivo carregado", "arquivo", a.Nome, "lidas", est.Lidas, "gravadas", est.Gravadas, "rejeitadas", est.Rejeitadas)
		return nil
	}); err != nil {
		return err
	}
	e.situacao(ctx, "carregando", "agregando a dívida ativa por CNPJ", 4)
	if err := executar(ctx, e.conn, `SET work_mem = '96MB'`, `SET maintenance_work_mem = '512MB'`, `CREATE SCHEMA IF NOT EXISTS pgfn`); err != nil {
		return err
	}
	if err := AgregarDivida(ctx, e.conn, EsquemaCarga, "pgfn.divida_nova"); err != nil {
		return err
	}
	if err := executar(ctx, e.conn, `BEGIN`,
		`DROP TABLE IF EXISTS pgfn.divida`,
		`ALTER TABLE pgfn.divida_nova RENAME TO divida`,
		`ALTER INDEX IF EXISTS pgfn.divida_nova_pkey RENAME TO divida_pkey`,
		`CREATE TABLE IF NOT EXISTS pgfn.meta (referencia text NOT NULL, carregada_em timestamptz NOT NULL DEFAULT now())`,
		`DELETE FROM pgfn.meta`,
		fmt.Sprintf(`INSERT INTO pgfn.meta (referencia) VALUES ('%s')`, strings.ReplaceAll(ref, "'", "")),
		`COMMIT`, `RESET work_mem`, `RESET maintenance_work_mem`); err != nil {
		_, _ = e.conn.Exec(context.WithoutCancel(ctx), `ROLLBACK`)
		return err
	}
	if _, err := e.pool.Exec(ctx, `TRUNCATE carga.pgfn`); err != nil {
		return err
	}
	e.limpar(ctx, fonte, ref)
	return nil
}

// limpar apaga os zips e o controle de uma referência já em uso.
func (e *execucao) limpar(ctx context.Context, fonte, ref string) {
	if err := os.RemoveAll(filepath.Dir(e.caminho(fonte, ref, "x"))); err != nil {
		slog.Warn("não foi possível apagar os arquivos baixados", "erro", err)
	}
	if _, err := e.pool.Exec(context.WithoutCancel(ctx), `DELETE FROM carga_arquivos WHERE fonte = $1`, fonte); err != nil {
		slog.Warn("não foi possível limpar o controle de arquivos", "erro", err)
	}
}

func (e *execucao) construirETrocar(ctx context.Context, atual metaAtual, esquemaCarga, refPGFN string) error {
	refCNPJ := atual.refCNPJ
	ufs := atual.ufs
	if esquemaCarga != "" {
		refCNPJ, ufs = e.refCNPJ, e.s.cfg.UFs
	}
	divida := ""
	if refPGFN != "" {
		divida = "pgfn.divida"
	}
	e.situacao(ctx, "construindo", "preparando", 76)
	err := Construir(ctx, e.conn, Construcao{
		Destino: EsquemaServico, Carga: esquemaCarga, Divida: divida,
		Versao:         NovaVersao(refCNPJ, refPGFN),
		ReferenciaCNPJ: refCNPJ, ReferenciaPGFN: refPGFN, UFs: ufs,
		Estatisticas: e.estatisticas(),
		Etapa: func(nome string, p int) {
			e.situacao(ctx, "construindo", nome, 76+p*22/100)
		},
	})
	if err != nil {
		return fmt.Errorf("construção da base: %w", err)
	}
	e.situacao(ctx, "construindo", "trocando a base", 98)
	if err := Trocar(ctx, e.conn, EsquemaServico); err != nil {
		return err
	}
	e.situacao(ctx, "construindo", "aquecendo", 99)
	Aquecer(ctx, e.conn, EsquemaServico)
	return nil
}

// NovaVersao gera a versão de uma base: muda a cada troca, e os cursores de
// busca a carregam.
func NovaVersao(refCNPJ, refPGFN string) string {
	return strings.Join(naoVazios(refCNPJ, refPGFN, fmt.Sprintf("%x", time.Now().UnixMilli())), ".")
}

// ---------------------------------------------------------------- situação

// EstadoCarga é o bloco "carga" de GET /v1/situacao.
type EstadoCarga struct {
	Estado     string          `json:"estado"`
	Fonte      string          `json:"fonte"`
	Referencia string          `json:"referencia"`
	Etapa      string          `json:"etapa"`
	Percentual int             `json:"percentual"`
	IniciadaEm *time.Time      `json:"iniciada_em"`
	Erro       string          `json:"erro"`
	Arquivos   []EstadoArquivo `json:"arquivos"`
}

// EstadoArquivo é um arquivo da carga em curso.
type EstadoArquivo struct {
	Nome       string `json:"nome"`
	Estado     string `json:"estado"`
	Bytes      int64  `json:"bytes"`
	TotalBytes int64  `json:"total_bytes"`
	Linhas     int64  `json:"linhas"`
}

// Situacao lê do banco a carga mais recente (a carga pode estar rodando em
// outro processo, pela flag -carga).
func Situacao(ctx context.Context, pool *pgxpool.Pool) (EstadoCarga, error) {
	c := EstadoCarga{Estado: "ociosa", Arquivos: []EstadoArquivo{}}
	var id int64
	var iniciada time.Time
	err := pool.QueryRow(ctx, `SELECT id, fonte, referencia, estado, etapa, percentual, erro, iniciada_em FROM cargas ORDER BY id DESC LIMIT 1`).
		Scan(&id, &c.Fonte, &c.Referencia, &c.Estado, &c.Etapa, &c.Percentual, &c.Erro, &iniciada)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	c.IniciadaEm = &iniciada
	if c.Estado == "ociosa" {
		return c, nil
	}
	linhas, err := pool.Query(ctx, `SELECT nome, estado, bytes, total_bytes, greatest(linhas, linhas_lidas) FROM carga_arquivos WHERE carga_id = $1 ORDER BY fonte, nome`, id)
	if err != nil {
		return c, err
	}
	defer linhas.Close()
	for linhas.Next() {
		var a EstadoArquivo
		if err := linhas.Scan(&a.Nome, &a.Estado, &a.Bytes, &a.TotalBytes, &a.Linhas); err != nil {
			return c, err
		}
		c.Arquivos = append(c.Arquivos, a)
	}
	return c, linhas.Err()
}
