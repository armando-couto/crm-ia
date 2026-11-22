package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services/enriquecimento"
)

// -----------------------------------------------------------------------------
// Job "enviar_crm" (docs/PROSPECCAO.md 8.1, com o adendo). Por item apto:
// bloqueios → empresa (cria, vincula ou pula) → contatos decisores → lead com
// telefones ordenados e resumo pronto → item "enviado". Idempotente por item:
// a empresa criada é gravada no item antes de seguir, o lead é único por
// item, os contatos não repetem nome, e o estado do job guarda o item em
// curso com o dono já escolhido. Nenhuma automação é disparada.
// -----------------------------------------------------------------------------

func init() { RegistrarProcessadorJob(models.JobEnviarCRM, processadorEnvio{}) }

// EnvioItensPorFatia é quantos itens cada fatia processa.
var EnvioItensPorFatia = 25

// ResultadoEnvio é o resultado do job (prospeccao_jobs.resultado).
type ResultadoEnvio struct {
	Leads              int               `json:"leads"`
	EmpresasCriadas    int               `json:"empresas_criadas"`
	EmpresasExistentes int               `json:"empresas_existentes"`
	Contatos           int               `json:"contatos"`
	Duplicados         int               `json:"duplicados"`
	Bloqueados         int               `json:"bloqueados"`
	Erros              int               `json:"erros"`
	PorUsuario         []EnvioPorUsuario `json:"por_usuario"`
}

// envioEmCurso é o item que estava sendo processado quando a fatia parou.
type envioEmCurso struct {
	ID             int64     `json:"id"`
	Dono           *int64    `json:"dono"`
	RodizioDepois  int       `json:"rodizio_depois"` // posição do rodízio se o lead nascer
	ProximaAcao    time.Time `json:"proxima_acao"`
	CriandoEmpresa bool      `json:"criando_empresa,omitempty"`
	EmpresaCriada  bool      `json:"empresa_criada,omitempty"`
	IntelGravada   bool      `json:"intel_gravada,omitempty"`
}

// estadoEnvio é o cursor de retomada.
type estadoEnvio struct {
	Inicio      time.Time `json:"inicio"`
	AposID      int64     `json:"apos_id"`
	ItemEmCurso int64     `json:"item_em_curso,omitempty"` // adendo: {"item_em_curso": id, "empresa_criada": true}
	// EmpresaCriada espelha Curso.EmpresaCriada no formato do adendo.
	EmpresaCriada bool           `json:"empresa_criada,omitempty"`
	Curso         *envioEmCurso  `json:"curso,omitempty"`
	Rodizio       int            `json:"rodizio"`
	PorDono       map[string]int `json:"por_dono"` // "id" (ou "0" sem dono) → leads recebidos
	Res           ResultadoEnvio `json:"res"`
}

func chaveDono(dono *int64) string {
	if dono == nil {
		return "0"
	}
	return fmt.Sprint(*dono)
}

type processadorEnvio struct{}

func (processadorEnvio) Fatia(ctx context.Context, e *ExecucaoJob) (Fatia, error) {
	var par parametrosEnvio
	var est estadoEnvio
	if err := e.Parametros(&par); err != nil {
		return Fatia{}, err
	}
	if err := e.Estado(&est); err != nil {
		return Fatia{}, err
	}
	if e.Job.ListaID == nil {
		return Fatia{Concluido: true, Mensagem: "a lista foi apagada antes do envio", Resultado: resultadoFinal(e.DB, est)}, nil
	}
	lista, err := models.ProspListaByID(e.DB, *e.Job.ListaID)
	if err == sql.ErrNoRows {
		return Fatia{Concluido: true, Mensagem: "a lista foi apagada antes do envio", Resultado: resultadoFinal(e.DB, est)}, nil
	}
	if err != nil {
		return Fatia{}, Transitorio(err)
	}
	if est.Inicio.IsZero() {
		est.Inicio = time.Now()
		if est.PorDono == nil {
			est.PorDono = map[string]int{}
		}
		if err := e.SalvarEstado(est); err != nil {
			return Fatia{}, Transitorio(err)
		}
	}
	if est.PorDono == nil {
		est.PorDono = map[string]int{}
	}
	cfg := models.LoadConfigProspeccao(e.DB)

	ids, err := models.ResolveProspSelecao(e.DB, lista.ID, par.Selecao, models.ItemEstadosAptosEnvio, est.AposID, EnvioItensPorFatia)
	if err != nil {
		return Fatia{}, Transitorio(err)
	}
	if len(ids) == 0 {
		return concluirEnvio(e, lista, est)
	}
	itens, err := models.ProspItensCompletos(e.DB, ids)
	if err != nil {
		return Fatia{}, Transitorio(err)
	}
	sort.Slice(itens, func(i, j int) bool { return itens[i].ID < itens[j].ID })
	ativos, err := usuariosAtivos(e.DB, par.Distribuicao.Usuarios)
	if err != nil {
		return Fatia{}, Transitorio(err)
	}
	for i := range itens {
		if e.Cancelado() {
			return Fatia{}, nil // o worker já não deixa reagendar nem concluir
		}
		it := &itens[i]
		if !aptoEnvio(it.Estado) {
			est.AposID = it.ID
			continue
		}
		// Dono e próxima ação escolhidos uma vez por item (sobrevivem a reinício).
		if est.Curso == nil || est.Curso.ID != it.ID {
			dono, depois := escolherDono(est.Rodizio, par.Distribuicao, ativos)
			indice := est.PorDono[chaveDono(dono)]
			est.Curso = &envioEmCurso{
				ID: it.ID, Dono: dono, RodizioDepois: depois,
				ProximaAcao: ProximaAcaoDistribuida(est.Inicio, time.Now(), indice, par.Distribuicao.PorDia, cfg.JanelaInicio),
			}
			est.ItemEmCurso, est.EmpresaCriada = it.ID, false
			if err := e.SalvarEstado(est); err != nil {
				return Fatia{}, Transitorio(err)
			}
		}
		desfecho, err := enviarItem(e, lista, par, &est, it)
		if err != nil {
			// O item fica apto, com o erro anotado; o envio segue com os outros.
			log.Printf("prospecção: envio do item %d ao CRM falhou: %v", it.ID, err)
			if e2 := models.SetProspItemEstado(e.DB, it.ID, models.ProspItemEstado{Estado: it.Estado,
				Erro: "falha ao enviar ao CRM: " + cortar(err.Error(), 200)}); e2 != nil {
				return Fatia{}, Transitorio(err)
			}
			est.Res.Erros++
		} else {
			switch desfecho {
			case models.ItemBloqueado:
				est.Res.Bloqueados++
			case models.ItemDuplicado:
				est.Res.Duplicados++
			case models.ItemEnviado:
				est.Res.Leads++
				est.PorDono[chaveDono(est.Curso.Dono)]++
				est.Rodizio = est.Curso.RodizioDepois // o rodízio só anda quando o lead nasce
			}
		}
		est.AposID, est.Curso, est.ItemEmCurso, est.EmpresaCriada = it.ID, nil, 0, false
		if err := e.SalvarEstado(est); err != nil {
			return Fatia{}, Transitorio(err)
		}
		erros := 0
		if err != nil {
			erros = 1
		}
		if err := e.Progresso(1-erros, erros, 0); err != nil {
			return Fatia{}, Transitorio(err)
		}
	}
	_ = e.Mensagem(fmt.Sprintf("enviando ao CRM: %d leads criados", est.Res.Leads))
	if len(ids) < EnvioItensPorFatia {
		return concluirEnvio(e, lista, est)
	}
	return Fatia{Mensagem: e.Job.Mensagem}, nil
}

func aptoEnvio(estado string) bool {
	for _, s := range models.ItemEstadosAptosEnvio {
		if s == estado {
			return true
		}
	}
	return false
}

// escolherDono aplica a distribuição: um vendedor só, ou rodízio em ordem
// fixa a partir da posição atual, pulando os inativos. Devolve o dono (nil
// sem ninguém ativo: o lead fica sem dono) e a posição seguinte do rodízio.
func escolherDono(posicao int, d DistribuicaoEnvio, ativos map[int64]bool) (*int64, int) {
	n := len(d.Usuarios)
	if n == 0 {
		return nil, 0
	}
	if d.Modo != DistribuicaoRodizio {
		u := d.Usuarios[0]
		if ativos[u] {
			return &u, 0
		}
		return nil, 0
	}
	for i := 0; i < n; i++ {
		u := d.Usuarios[(posicao+i)%n]
		if ativos[u] {
			return &u, (posicao + i + 1) % n
		}
	}
	return nil, posicao
}

// leadDoItem devolve o lead já criado para o item (retomada), se houver.
func leadDoItem(db models.Executor, itemID int64) (*int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM leads WHERE item_id = $1`, itemID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// enviarItem processa um item e devolve o estado final dele (enviado,
// duplicado ou bloqueado).
func enviarItem(e *ExecucaoJob, lista *models.ProspLista, par parametrosEnvio, est *estadoEnvio, it *models.ProspItem) (string, error) {
	db := e.DB
	curso := est.Curso
	userID := e.UserID()

	// Retomada depois de o lead já ter nascido: só falta fechar o item.
	if leadID, err := leadDoItem(db, it.ID); err != nil {
		return "", err
	} else if leadID != nil {
		return models.ItemEnviado, models.SetProspItemEstado(db, it.ID, models.ProspItemEstado{
			Estado: models.ItemEnviado, CompanyID: it.CompanyID, LeadID: leadID})
	}

	// 1. Bloqueios.
	contatos, bloqueado, _, err := conferirContatosDoItem(db, it)
	if err != nil {
		return "", err
	}
	if bloqueado {
		return models.ItemBloqueado, models.SetProspItemEstado(db, it.ID, models.ProspItemEstado{Estado: models.ItemBloqueado})
	}

	// 2. Empresa.
	var company *models.Company
	nossa := false // criada por este envio (agora ou antes de um reinício)
	if it.CompanyID != nil {
		// Item apto com empresa gravada: foi este envio que a criou (adendo).
		c, err := models.CompanyByID(db, *it.CompanyID)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
		if c != nil {
			company, nossa = c, true
		}
	}
	if company == nil && curso.CriandoEmpresa {
		// Reinício entre criar a empresa e gravá-la no item: adota a que nasceu
		// depois do início do job com o mesmo CNPJ.
		c, err := models.CompanyByCNPJ(db, it.CNPJ)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
		if c != nil && !c.CreatedAt.Before(est.Inicio.Add(-time.Minute)) {
			if err := models.SetProspItemEmpresa(db, it.ID, c.ID); err != nil {
				return "", err
			}
			company, nossa = c, true
		}
	}
	perfis := perfisDoItem(it, contatos)
	mesclado := enriquecimento.Mesclar(perfis...)
	if company == nil {
		existente, err := models.CompanyByCNPJ(db, it.CNPJ)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
		if existente != nil {
			if par.Duplicados != DuplicadosVincular {
				return models.ItemDuplicado, models.SetProspItemEstado(db, it.ID, models.ProspItemEstado{
					Estado: models.ItemDuplicado, CompanyID: &existente.ID})
			}
			aberto, err := models.LeadAbertoDaEmpresa(db, existente.ID)
			if err != nil {
				return "", err
			}
			if aberto != nil {
				return models.ItemDuplicado, models.SetProspItemEstado(db, it.ID, models.ProspItemEstado{
					Estado: models.ItemDuplicado, CompanyID: &existente.ID, LeadID: aberto})
			}
			company = existente
			est.Res.EmpresasExistentes++
			atividadeDoEnvio(db, models.Activity{Kind: models.ActivitySistema, UserID: userID, CompanyID: &company.ID,
				Content: fmt.Sprintf("Lead criado pela lista de prospecção %s", lista.Nome)})
		} else {
			curso.CriandoEmpresa = true
			if err := e.SalvarEstado(est); err != nil {
				return "", err
			}
			c := empresaDoItem(it, mesclado, curso.Dono)
			if err := models.CreateCompany(db, c); err != nil {
				return "", err
			}
			if err := models.SetProspItemEmpresa(db, it.ID, c.ID); err != nil {
				return "", err
			}
			curso.EmpresaCriada, est.EmpresaCriada = true, true
			if err := e.SalvarEstado(est); err != nil {
				return "", err
			}
			company, nossa = c, true
		}
	}
	// Inteligência, fontes e atividade da empresa criada: uma vez só.
	if nossa && (curso.CriandoEmpresa || curso.EmpresaCriada) && !curso.IntelGravada {
		est.Res.EmpresasCriadas++
		if err := gravarIntelDoEnvio(db, company.ID, perfis, mesclado, userID); err != nil {
			return "", err
		}
		atividadeDoEnvio(db, models.Activity{Kind: models.ActivitySistema, UserID: userID, CompanyID: &company.ID,
			Content: fmt.Sprintf("Empresa criada pela lista de prospecção %s", lista.Nome)})
		curso.IntelGravada = true
		if err := e.SalvarEstado(est); err != nil {
			return "", err
		}
	}
	if par.ContaAlvoTier > 0 && !company.IsTarget {
		if err := models.SetTargetAccount(db, company.ID, true, par.ContaAlvoTier, ""); err != nil {
			return "", err
		}
	}

	// 3. Contatos decisores (nunca em lista de pessoa física sem incluir_pf).
	var contactID *int64
	if par.CriarContatos && !(it.PessoaFisica && !lista.IncluirPF) {
		principal, criados, err := criarContatosDoItem(db, company.ID, curso.Dono, candidatosAContato(it, contatos))
		if err != nil {
			return "", err
		}
		contactID = principal
		est.Res.Contatos += criados
	}

	// 4. Lead.
	titulo := strings.TrimSpace(it.NomeFantasia)
	if titulo == "" {
		titulo = strings.TrimSpace(it.RazaoSocial)
	}
	if titulo == "" {
		titulo = company.Name
	}
	proxima := curso.ProximaAcao
	if proxima.IsZero() {
		proxima = time.Now()
	}
	cidade := NomeProprio(it.Municipio)
	if cidade == "" {
		cidade = company.City
	}
	uf := it.UF
	if uf == "" {
		uf = company.State
	}
	listaID, itemID, companyID := lista.ID, it.ID, company.ID
	lead := &models.Lead{
		Titulo:        titulo,
		CompanyID:     &companyID,
		ContactID:     contactID,
		CNPJ:          it.CNPJ,
		Cidade:        cidade,
		UF:            uf,
		Origem:        models.LeadOrigemProspeccao,
		ListaID:       &listaID,
		ItemID:        &itemID,
		OwnerID:       curso.Dono,
		Estado:        models.LeadNovo,
		Temperatura:   par.Temperatura,
		Etiquetas:     par.Etiquetas,
		Nota:          it.Nota,
		Telefones:     contatos.telefones,
		Emails:        contatos.emails,
		Resumo:        MontarResumoLead(it.Dados, it.Enriquecimento, contatos.telefones, time.Now()),
		ProximaAcaoEm: &proxima,
		PessoaFisica:  it.PessoaFisica,
		CriadoPor:     userID,
	}
	if _, err := models.CreateLead(db, lead); err != nil {
		return "", err
	}

	// 5. Item enviado.
	return models.ItemEnviado, models.SetProspItemEstado(db, it.ID, models.ProspItemEstado{
		Estado: models.ItemEnviado, CompanyID: &companyID, LeadID: &lead.ID})
}

// atividadeDoEnvio grava a atividade sem derrubar o envio quando falha.
func atividadeDoEnvio(db *sql.DB, a models.Activity) {
	if err := models.CreateActivity(db, &a); err != nil {
		log.Printf("prospecção: falha ao registrar atividade: %v", err)
	}
}

// resultadoFinal monta o resultado com os nomes dos vendedores.
func resultadoFinal(db *sql.DB, est estadoEnvio) ResultadoEnvio {
	res := est.Res
	res.PorUsuario = []EnvioPorUsuario{}
	ids := []int64{}
	for k := range est.PorDono {
		var id int64
		if _, err := fmt.Sscan(k, &id); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	nomes, _ := nomesDosUsuarios(db, ids)
	for k, q := range est.PorDono {
		var id int64
		_, _ = fmt.Sscan(k, &id)
		nome := nomes[id]
		if id == 0 {
			nome = "Sem dono"
		}
		res.PorUsuario = append(res.PorUsuario, EnvioPorUsuario{UserID: id, Nome: nome, Quantidade: q})
	}
	sort.Slice(res.PorUsuario, func(i, j int) bool {
		if res.PorUsuario[i].Quantidade != res.PorUsuario[j].Quantidade {
			return res.PorUsuario[i].Quantidade > res.PorUsuario[j].Quantidade
		}
		return res.PorUsuario[i].UserID < res.PorUsuario[j].UserID
	})
	return res
}

func mensagemEnvio(res ResultadoEnvio) string {
	plural := func(n int, um, varios string) string {
		if n == 1 {
			return "1 " + um
		}
		return fmt.Sprintf("%d %s", n, varios)
	}
	partes := []string{plural(res.Leads, "lead criado", "leads criados")}
	if res.EmpresasCriadas > 0 {
		partes = append(partes, plural(res.EmpresasCriadas, "empresa nova", "empresas novas"))
	}
	if res.Contatos > 0 {
		partes = append(partes, plural(res.Contatos, "contato", "contatos"))
	}
	if res.Duplicados > 0 {
		partes = append(partes, plural(res.Duplicados, "já estava no CRM", "já estavam no CRM"))
	}
	if res.Bloqueados > 0 {
		partes = append(partes, plural(res.Bloqueados, "bloqueado", "bloqueados"))
	}
	if res.Erros > 0 {
		partes = append(partes, plural(res.Erros, "com erro", "com erro"))
	}
	return strings.Join(partes, ", ")
}

// concluirEnvio fecha o job com o resultado e a auditoria "lista enviada ao CRM".
func concluirEnvio(e *ExecucaoJob, lista *models.ProspLista, est estadoEnvio) (Fatia, error) {
	res := resultadoFinal(e.DB, est)
	msg := mensagemEnvio(res)
	auditarEnvio(e, lista, res, "lista enviada ao CRM")
	return Fatia{Concluido: true, Mensagem: msg, Resultado: res}, nil
}

// auditarEnvio grava a trilha; falha de auditoria não derruba o job.
func auditarEnvio(e *ExecucaoJob, lista *models.ProspLista, res ResultadoEnvio, titulo string) {
	detalhes, _ := json.Marshal(res)
	entrada := models.AuditEntry{
		UserID: e.UserID(), Action: models.AuditBulk, Entity: EntidadeListaProspeccao, EntityID: &lista.ID,
		Summary: fmt.Sprintf("%s: %q, %s", titulo, lista.Nome, mensagemEnvio(res)), Details: detalhes,
	}
	if entrada.UserID != nil {
		_ = e.DB.QueryRow(`SELECT name FROM users WHERE id = $1`, *entrada.UserID).Scan(&entrada.UserName)
	}
	if err := models.RecordAudit(e.DB, &entrada); err != nil {
		log.Printf("prospecção: falha ao registrar auditoria do envio da lista %d: %v", lista.ID, err)
	}
}

// Encerrar registra o resultado parcial quando o envio é cancelado ou falha:
// o que já foi enviado fica (leads, empresas e contatos criados).
func (processadorEnvio) Encerrar(ctx context.Context, e *ExecucaoJob, status string) (string, any, error) {
	var est estadoEnvio
	_ = e.Estado(&est)
	res := resultadoFinal(e.DB, est)
	motivo := "envio cancelado"
	if status == models.JobFalhou {
		motivo = "envio interrompido por erro"
	}
	if e.Job.ListaID != nil {
		if lista, err := models.ProspListaByID(e.DB, *e.Job.ListaID); err == nil {
			auditarEnvio(e, lista, res, "lista enviada ao CRM em parte ("+motivo+")")
		}
	}
	return motivo + ": " + mensagemEnvio(res), res, nil
}
