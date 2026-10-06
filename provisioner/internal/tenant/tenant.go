// Package tenant orquestra o ciclo de vida do ambiente de cada cliente do
// CRM IA: PostgreSQL e Redis dedicados mais a aplicação (API + SPA), como uma
// stack Compose própria, roteada pelo Traefik em https://<dominio>/<slug>.
package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	// text/template, e não html/template: o alvo é YAML — o html escaparia
	// aspas e & dentro de senhas e corromperia o compose.
	"text/template"
	"time"

	"github.com/armando-couto/crm-ia/provisioner/internal/config"
)

// prefixo dos containers e volumes: ci-<slug>-db, ci-<slug>-redis, ci-<slug>-app…
const prefixo = "ci-"

type Estado string

const (
	EstadoAusente  Estado = "ausente"
	EstadoAtivo    Estado = "ativo"
	EstadoSuspenso Estado = "suspenso"
	EstadoParado   Estado = "parado"
	EstadoErro     Estado = "erro"
)

// Pedido é o que o painel manda para criar (ou recriar) um ambiente.
type Pedido struct {
	Slug       string `json:"slug"`
	Nome       string `json:"nome"`
	CNPJ       string `json:"cnpj"`
	AdminNome  string `json:"admin_nome"`
	AdminEmail string `json:"admin_email"`
	// Versao é a tag das imagens. Vazio = padrão da plataforma.
	Versao      string `json:"versao"`
	UsuariosMax int    `json:"usuarios_max"`
	CorPrimaria string `json:"cor_primaria"`
	LogoURL     string `json:"logo_url"`
}

// Atualizacao é o que pode mudar num ambiente no ar sem recriá-lo do zero.
// Ponteiro nil = não mexer.
type Atualizacao struct {
	Versao      string  `json:"versao"`
	UsuariosMax *int    `json:"usuarios_max,omitempty"`
	CorPrimaria *string `json:"cor_primaria,omitempty"`
	LogoURL     *string `json:"logo_url,omitempty"`
	Nome        *string `json:"nome,omitempty"`
}

type ContainerInfo struct {
	Nome   string `json:"nome"`
	Estado string `json:"estado"`
	Status string `json:"status"`
	Imagem string `json:"imagem"`
}

type VersaoInfo struct {
	Configurada string `json:"configurada"`
	EmExecucao  string `json:"em_execucao"`
}

type Resultado struct {
	Slug   string `json:"slug"`
	Status Estado `json:"status"`
	URL    string `json:"url"`
	Versao string `json:"versao"`
	Banco  string `json:"banco,omitempty"`
	// SenhaAdmin volta só no primeiro provisionamento e nunca é gravada no
	// painel: ele a exibe uma única vez.
	SenhaAdmin string          `json:"senha_admin,omitempty"`
	Containers []ContainerInfo `json:"containers,omitempty"`
	Versoes    VersaoInfo      `json:"versoes"`
	Erro       string          `json:"erro,omitempty"`
}

// segredosTenant é o que não pode mudar entre provisionamentos: regerar a
// chave de cifra tornaria ilegível o que já está cifrado no banco do cliente;
// regerar o JWT deslogaria a empresa inteira.
type segredosTenant struct {
	BancoSenha   string    `json:"banco_senha"`
	JWTSegredo   string    `json:"jwt_segredo"`
	ChaveCripto  string    `json:"chave_cripto"`
	TokenInterno string    `json:"token_interno"`
	CriadoEm     time.Time `json:"criado_em"`
	// SenhaAdminPendente fica guardada até o provisionamento terminar bem:
	// uma tentativa que falha depois de o app já ter criado o administrador
	// precisa repetir a MESMA senha na próxima.
	SenhaAdminPendente string `json:"senha_admin_pendente,omitempty"`
	AdminEntregue      bool   `json:"admin_entregue"`
}

type Servico struct {
	cfg             Config
	template        *template.Template
	intervaloEspera time.Duration

	// Um cliente por vez: duas operações no mesmo slug disputariam o compose.
	mu     sync.Mutex
	travas map[string]*sync.Mutex
}

// Config é a configuração que o serviço usa; DockerBin permite trocar o
// binário nos testes.
type Config struct {
	config.Config
	Docker string
}

func (c Config) DockerBin() string {
	if c.Docker != "" {
		return c.Docker
	}
	return "docker"
}

func NovoServico(cfg Config) (*Servico, error) {
	t, err := template.New(filepath.Base(cfg.TemplatePath)).ParseFiles(cfg.TemplatePath)
	if err != nil {
		return nil, fmt.Errorf("lendo template do tenant: %w", err)
	}
	for _, d := range []string{cfg.TenantsDir, cfg.BackupsDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, fmt.Errorf("criando diretório %s: %w", d, err)
		}
	}
	espera := cfg.EsperaHealthcheck
	if espera <= 0 {
		espera = 2 * time.Second
	}
	return &Servico{cfg: cfg, template: t, intervaloEspera: espera, travas: map[string]*sync.Mutex{}}, nil
}

func (s *Servico) trava(slug string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.travas[slug]; ok {
		return t
	}
	t := &sync.Mutex{}
	s.travas[slug] = t
	return t
}

func (s *Servico) dirTenant(slug string) string { return filepath.Join(s.cfg.TenantsDir, slug) }
func (s *Servico) caminhoCompose(slug string) string {
	return filepath.Join(s.dirTenant(slug), "docker-compose.yml")
}
func (s *Servico) caminhoSegredos(slug string) string {
	return filepath.Join(s.dirTenant(slug), "segredos.json")
}
func (s *Servico) caminhoPedido(slug string) string {
	return filepath.Join(s.dirTenant(slug), "pedido.json")
}

func (s *Servico) nomeBanco(slug string) string {
	return "crmia_" + strings.ReplaceAll(slug, "-", "_")
}

var slugValido = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?$`)

// reservados são caminhos da plataforma que um cliente não pode ocupar.
var reservados = map[string]bool{"painel": true, "api": true, "assets": true, "static": true, "health": true, "admin": true, "login": true, "www": true, "app": true}

func ValidarSlug(slug string) error {
	if !slugValido.MatchString(slug) {
		return errors.New("slug inválido: use de 1 a 40 caracteres entre letras minúsculas, números e hífen, começando e terminando com letra ou número")
	}
	if reservados[slug] {
		return fmt.Errorf("o endereço %q é reservado pela plataforma", slug)
	}
	return nil
}

func (s *Servico) versaoOuPadrao(v string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return s.cfg.VersaoPadrao
}

func (s *Servico) urlPublica(slug string) string {
	dominio := s.cfg.Dominio
	if strings.HasPrefix(dominio, "http://") || strings.HasPrefix(dominio, "https://") {
		return strings.TrimRight(dominio, "/") + "/" + slug
	}
	return "https://" + dominio + "/" + slug
}

// -----------------------------------------------------------------------------
// Arquivos do tenant
// -----------------------------------------------------------------------------

func (s *Servico) salvarPedido(p Pedido) error {
	if err := os.MkdirAll(s.dirTenant(p.Slug), 0o750); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(s.caminhoPedido(p.Slug), b, 0o600)
}

func (s *Servico) carregarPedido(slug string) (Pedido, error) {
	var p Pedido
	b, err := os.ReadFile(s.caminhoPedido(slug))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return p, fmt.Errorf("cliente %s ainda não foi provisionado", slug)
		}
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("pedido de %s corrompido: %w", slug, err)
	}
	return p, nil
}

func (s *Servico) carregarOuGerarSegredos(slug string) (segredosTenant, bool, error) {
	var seg segredosTenant
	b, err := os.ReadFile(s.caminhoSegredos(slug))
	if err == nil {
		if err := json.Unmarshal(b, &seg); err != nil {
			return seg, false, fmt.Errorf("segredos de %s corrompidos: %w", slug, err)
		}
		return seg, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return seg, false, err
	}
	senhaBanco, err := segredo(28)
	if err != nil {
		return seg, false, err
	}
	jwt, err := chaveHex(32)
	if err != nil {
		return seg, false, err
	}
	cripto, err := chaveHex(32)
	if err != nil {
		return seg, false, err
	}
	interno, err := chaveHex(32)
	if err != nil {
		return seg, false, err
	}
	senhaAdmin, err := segredo(12)
	if err != nil {
		return seg, false, err
	}
	seg = segredosTenant{BancoSenha: senhaBanco, JWTSegredo: jwt, ChaveCripto: cripto, TokenInterno: interno,
		CriadoEm: time.Now(), SenhaAdminPendente: senhaAdmin}
	return seg, true, s.gravarSegredos(slug, seg)
}

func (s *Servico) gravarSegredos(slug string, seg segredosTenant) error {
	if err := os.MkdirAll(s.dirTenant(slug), 0o750); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(seg, "", "  ")
	tmp := s.caminhoSegredos(slug) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.caminhoSegredos(slug))
}

// dadosTemplate é o que o template do compose consome.
type dadosTemplate struct {
	Slug, Nome, CNPJ, Dominio, URL, RedePublica, GeradoEm, Versao string
	ImagemApp, ImagemDB, ImagemRedis, ImagemSuspenso              string
	CPULimite, MemLimite, DBMemLimite, RedisMemLimite             string
	BancoNome, BancoUsuario, BancoSenha                           string
	JWTSegredo, ChaveCripto, TokenInterno                         string
	PainelURL                                                     string
	UsuariosMax                                                   int
	CorPrimaria, LogoURL                                          string
	AdminNome, AdminEmail, AdminSenha                             string
	MandrillChave, FromEmail, FromNome                            string
}

func (s *Servico) gerarCompose(p Pedido, seg segredosTenant, senhaAdmin string) error {
	banco := s.nomeBanco(p.Slug)
	adminNome := p.AdminNome
	if adminNome == "" {
		adminNome = "Administrador"
	}
	adminEmail := p.AdminEmail
	if adminEmail == "" {
		adminEmail = "admin@" + p.Slug + ".local"
	}
	d := dadosTemplate{
		Slug: p.Slug, Nome: escaparYAML(p.Nome), CNPJ: p.CNPJ, Dominio: s.cfg.Dominio, URL: s.urlPublica(p.Slug),
		RedePublica: s.cfg.RedePublica, GeradoEm: time.Now().Format("02/01/2006 15:04:05"), Versao: p.Versao,
		ImagemApp: s.cfg.ImagemApp, ImagemDB: s.cfg.ImagemDB, ImagemRedis: s.cfg.ImagemRedis, ImagemSuspenso: s.cfg.ImagemSuspenso,
		CPULimite: s.cfg.CPULimite, MemLimite: s.cfg.MemLimite, DBMemLimite: s.cfg.DBMemLimite, RedisMemLimite: s.cfg.RedisMemLimite,
		BancoNome: banco, BancoUsuario: banco, BancoSenha: escaparYAML(seg.BancoSenha),
		JWTSegredo: seg.JWTSegredo, ChaveCripto: seg.ChaveCripto, TokenInterno: seg.TokenInterno,
		PainelURL:   s.cfg.PainelURL,
		UsuariosMax: p.UsuariosMax, CorPrimaria: p.CorPrimaria, LogoURL: escaparYAML(p.LogoURL),
		AdminNome: escaparYAML(adminNome), AdminEmail: adminEmail, AdminSenha: escaparYAML(senhaAdmin),
		MandrillChave: escaparYAML(s.cfg.MandrillChave), FromEmail: s.cfg.FromEmail, FromNome: escaparYAML(s.cfg.FromNome),
	}
	if err := os.MkdirAll(s.dirTenant(p.Slug), 0o750); err != nil {
		return err
	}
	arquivo, err := os.OpenFile(s.caminhoCompose(p.Slug), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("criando arquivo de compose: %w", err)
	}
	defer arquivo.Close()
	if err := s.template.Execute(arquivo, d); err != nil {
		return fmt.Errorf("renderizando compose de %s: %w", p.Slug, err)
	}
	return nil
}

// escaparYAML protege valores que vão entre aspas duplas no YAML.
func escaparYAML(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return strings.ReplaceAll(v, "\n", `\n`)
}

// -----------------------------------------------------------------------------
// Ciclo de vida
// -----------------------------------------------------------------------------

// Criar provisiona (ou recria) o ambiente: segredos, compose, imagens, `up` e
// espera a aplicação ficar saudável. Devolve a senha do administrador só na
// primeira vez.
func (s *Servico) Criar(ctx context.Context, p Pedido) (Resultado, error) {
	if err := ValidarSlug(p.Slug); err != nil {
		return Resultado{Slug: p.Slug, Status: EstadoErro}, err
	}
	p.Versao = s.versaoOuPadrao(p.Versao)
	if p.UsuariosMax <= 0 {
		p.UsuariosMax = 5
	}
	if p.CorPrimaria == "" {
		p.CorPrimaria = "#6d5df6"
	}
	if strings.TrimSpace(p.Nome) == "" {
		p.Nome = p.Slug
	}
	t := s.trava(p.Slug)
	t.Lock()
	defer t.Unlock()

	res := Resultado{Slug: p.Slug, Status: EstadoErro, URL: s.urlPublica(p.Slug), Banco: s.nomeBanco(p.Slug), Versao: p.Versao}

	if s.estadoContainer(ctx, prefixo+p.Slug+"-suspenso") == "running" {
		return res, fmt.Errorf("o ambiente de %q está suspenso; reative antes de recriar", p.Slug)
	}
	seg, primeiraVez, err := s.carregarOuGerarSegredos(p.Slug)
	if err != nil {
		return res, err
	}
	senhaAdmin := ""
	if !seg.AdminEntregue {
		senhaAdmin = seg.SenhaAdminPendente
	}
	slog.Info("provisionando cliente", "slug", p.Slug, "versao", p.Versao, "primeira_vez", primeiraVez)

	if err := s.garantirImagensDaVersao(ctx, p.Versao); err != nil {
		return res, err
	}
	if err := s.salvarPedido(p); err != nil {
		return res, err
	}
	if err := s.gerarCompose(p, seg, senhaAdmin); err != nil {
		return res, err
	}
	if _, err := s.compose(ctx, p.Slug, "up", "-d", "--remove-orphans", "db", "redis", "app"); err != nil {
		_, _ = s.compose(ctx, p.Slug, "stop", "app")
		return res, err
	}
	if err := s.esperarSaudavel(ctx, prefixo+p.Slug+"-app", 3*time.Minute); err != nil {
		return res, fmt.Errorf("a aplicação do cliente não subiu: %w", err)
	}
	if senhaAdmin != "" {
		seg.AdminEntregue, seg.SenhaAdminPendente = true, ""
		if err := s.gravarSegredos(p.Slug, seg); err != nil {
			slog.Error("não consegui marcar a senha do admin como entregue", "slug", p.Slug, "erro", err)
		}
	}
	res.Status = EstadoAtivo
	res.SenhaAdmin = senhaAdmin
	res.Containers = s.containersDoTenant(ctx, p.Slug)
	res.Versoes = VersaoInfo{Configurada: p.Versao, EmExecucao: s.versaoEmExecucao(ctx, p.Slug)}
	slog.Info("cliente provisionado", "slug", p.Slug, "url", res.URL)
	return res, nil
}

// Atualizar recria só o container da aplicação com a versão/configuração nova.
// O banco e o Redis não são tocados. Nada aqui roda por rotina: é sempre uma
// decisão de quem administra (ou a política de auto-atualização no painel).
func (s *Servico) Atualizar(ctx context.Context, slug string, a Atualizacao) (Resultado, error) {
	if err := ValidarSlug(slug); err != nil {
		return Resultado{}, err
	}
	t := s.trava(slug)
	t.Lock()
	defer t.Unlock()

	p, err := s.carregarPedido(slug)
	if err != nil {
		return Resultado{}, err
	}
	seg, primeiraVez, err := s.carregarOuGerarSegredos(slug)
	if err != nil {
		return Resultado{}, err
	}
	if primeiraVez {
		return Resultado{}, fmt.Errorf("cliente %s ainda não foi provisionado", slug)
	}
	if s.estadoContainer(ctx, prefixo+slug+"-suspenso") == "running" {
		return Resultado{}, fmt.Errorf("o ambiente de %q está suspenso; reative antes de atualizar", slug)
	}
	if v := strings.TrimSpace(a.Versao); v != "" {
		p.Versao = v
	}
	if a.UsuariosMax != nil && *a.UsuariosMax > 0 {
		p.UsuariosMax = *a.UsuariosMax
	}
	if a.CorPrimaria != nil && *a.CorPrimaria != "" {
		p.CorPrimaria = *a.CorPrimaria
	}
	if a.LogoURL != nil {
		p.LogoURL = *a.LogoURL
	}
	if a.Nome != nil && strings.TrimSpace(*a.Nome) != "" {
		p.Nome = *a.Nome
	}
	if err := s.garantirImagensDaVersao(ctx, p.Versao); err != nil {
		return Resultado{}, err
	}
	senhaAdmin := ""
	if !seg.AdminEntregue {
		senhaAdmin = seg.SenhaAdminPendente
	}
	if err := s.salvarPedido(p); err != nil {
		return Resultado{}, err
	}
	if err := s.gerarCompose(p, seg, senhaAdmin); err != nil {
		return Resultado{}, err
	}
	if _, err := s.compose(ctx, slug, "up", "-d", "--force-recreate", "app"); err != nil {
		return Resultado{}, err
	}
	if err := s.esperarSaudavel(ctx, prefixo+slug+"-app", 2*time.Minute); err != nil {
		return Resultado{}, err
	}
	slog.Info("ambiente atualizado", "slug", slug, "versao", p.Versao)
	return s.Status(ctx, slug)
}

// Suspender bloqueia o acesso sem tocar em dado nenhum: sobe a página de
// licença suspensa (prioridade maior no Traefik) e para a aplicação. Banco e
// Redis continuam de pé.
func (s *Servico) Suspender(ctx context.Context, slug string) error {
	if err := ValidarSlug(slug); err != nil {
		return err
	}
	t := s.trava(slug)
	t.Lock()
	defer t.Unlock()
	if _, err := os.Stat(s.caminhoCompose(slug)); err != nil {
		return fmt.Errorf("cliente %s não está provisionado", slug)
	}
	if err := s.garantirImagem(ctx, s.cfg.ImagemSuspenso); err != nil {
		return fmt.Errorf("página de suspensão: %w", err)
	}
	if _, err := s.compose(ctx, slug, "--profile", "suspenso", "up", "-d", "suspenso"); err != nil {
		return fmt.Errorf("subindo página de suspensão: %w", err)
	}
	if _, err := s.compose(ctx, slug, "stop", "app"); err != nil {
		return fmt.Errorf("parando a aplicação: %w", err)
	}
	slog.Warn("cliente suspenso", "slug", slug)
	return nil
}

// Reativar sobe a aplicação de volta e remove a página de suspensão.
func (s *Servico) Reativar(ctx context.Context, slug string) error {
	if err := ValidarSlug(slug); err != nil {
		return err
	}
	t := s.trava(slug)
	t.Lock()
	defer t.Unlock()
	p, err := s.carregarPedido(slug)
	if err != nil {
		return err
	}
	if err := s.garantirImagensDaVersao(ctx, p.Versao); err != nil {
		return err
	}
	if _, err := s.compose(ctx, slug, "up", "-d", "db", "redis", "app"); err != nil {
		return fmt.Errorf("subindo a aplicação: %w", err)
	}
	if err := s.esperarSaudavel(ctx, prefixo+slug+"-app", 2*time.Minute); err != nil {
		return err
	}
	if _, err := s.compose(ctx, slug, "--profile", "suspenso", "rm", "-sf", "suspenso"); err != nil {
		slog.Warn("página de suspensão não pôde ser removida", "slug", slug, "erro", err)
	}
	slog.Info("cliente reativado", "slug", slug)
	return nil
}

// Backup faz um pg_dump do banco do cliente para o diretório de backups.
func (s *Servico) Backup(ctx context.Context, slug string) (string, error) {
	if err := ValidarSlug(slug); err != nil {
		return "", err
	}
	if _, err := os.Stat(s.caminhoCompose(slug)); err != nil {
		return "", fmt.Errorf("cliente %s não está provisionado", slug)
	}
	banco := s.nomeBanco(slug)
	arquivo := filepath.Join(s.cfg.BackupsDir, fmt.Sprintf("%s-%s.sql.gz", slug, time.Now().Format("20060102-150405")))
	// O dump roda dentro do container do banco; a saída vem pelo stdout.
	saida, err := s.dockerCom(ctx, 30*time.Minute, "", "exec", prefixo+slug+"-db", "sh", "-c",
		fmt.Sprintf("pg_dump -U %s %s | gzip", banco, banco))
	if err != nil {
		return "", fmt.Errorf("backup de %s: %w", slug, err)
	}
	if err := os.WriteFile(arquivo, []byte(saida), 0o600); err != nil {
		return "", err
	}
	slog.Info("backup concluído", "slug", slug, "arquivo", arquivo)
	return arquivo, nil
}

// Remover apaga o ambiente — containers, volumes e segredos — com backup
// obrigatório antes. A rotina de inadimplência nunca chega aqui: ela só suspende.
func (s *Servico) Remover(ctx context.Context, slug string) error {
	if err := ValidarSlug(slug); err != nil {
		return err
	}
	t := s.trava(slug)
	t.Lock()
	defer t.Unlock()
	if _, err := os.Stat(s.caminhoCompose(slug)); err != nil {
		return fmt.Errorf("cliente %s não está provisionado", slug)
	}
	if s.estadoContainer(ctx, prefixo+slug+"-db") == "running" {
		if _, err := s.Backup(ctx, slug); err != nil {
			return fmt.Errorf("sem backup não há remoção: %w", err)
		}
	}
	if _, err := s.compose(ctx, slug, "--profile", "suspenso", "down", "--volumes", "--remove-orphans"); err != nil {
		return err
	}
	if err := os.RemoveAll(s.dirTenant(slug)); err != nil {
		return err
	}
	slog.Warn("cliente removido", "slug", slug)
	return nil
}

// Status lê o estado real dos containers.
func (s *Servico) Status(ctx context.Context, slug string) (Resultado, error) {
	if err := ValidarSlug(slug); err != nil {
		return Resultado{}, err
	}
	res := Resultado{Slug: slug, URL: s.urlPublica(slug), Banco: s.nomeBanco(slug)}
	p, err := s.carregarPedido(slug)
	if err != nil {
		res.Status = EstadoAusente
		return res, nil
	}
	res.Versao = p.Versao
	res.Containers = s.containersDoTenant(ctx, slug)
	res.Versoes = VersaoInfo{Configurada: p.Versao, EmExecucao: s.versaoEmExecucao(ctx, slug)}
	switch {
	case s.estadoContainer(ctx, prefixo+slug+"-suspenso") == "running":
		res.Status = EstadoSuspenso
	case s.estadoContainer(ctx, prefixo+slug+"-app") == "running":
		res.Status = EstadoAtivo
	case len(res.Containers) == 0:
		res.Status = EstadoAusente
	default:
		res.Status = EstadoParado
	}
	return res, nil
}

// Listar devolve todos os ambientes conhecidos (pelo diretório de tenants).
func (s *Servico) Listar(ctx context.Context) ([]Resultado, error) {
	entradas, err := os.ReadDir(s.cfg.TenantsDir)
	if err != nil {
		return nil, err
	}
	var lista []Resultado
	for _, e := range entradas {
		if !e.IsDir() || ValidarSlug(e.Name()) != nil {
			continue
		}
		r, err := s.Status(ctx, e.Name())
		if err != nil {
			continue
		}
		lista = append(lista, r)
	}
	return lista, nil
}

// TokenInterno devolve o token que o painel usa para falar com o app do cliente.
func (s *Servico) TokenInterno(slug string) (string, error) {
	if err := ValidarSlug(slug); err != nil {
		return "", err
	}
	b, err := os.ReadFile(s.caminhoSegredos(slug))
	if err != nil {
		return "", fmt.Errorf("cliente %s ainda não foi provisionado", slug)
	}
	var seg segredosTenant
	if err := json.Unmarshal(b, &seg); err != nil {
		return "", err
	}
	return seg.TokenInterno, nil
}

// VersoesDisponiveis lista as tags da imagem do app presentes no host.
func (s *Servico) VersoesDisponiveis(ctx context.Context) ([]string, error) {
	saida, err := s.docker(ctx, "image", "ls", s.cfg.ImagemApp, "--format", "{{.Tag}}")
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, l := range strings.Split(strings.TrimSpace(saida), "\n") {
		if l = strings.TrimSpace(l); l != "" && l != "<none>" {
			tags = append(tags, l)
		}
	}
	return tags, nil
}
