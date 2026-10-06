package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// SetupStatus é o que a tela do assistente lê para saber onde o cliente parou.
type SetupStatus struct {
	Completed   bool                     `json:"completed"`
	Template    string                   `json:"template"`
	Steps       []string                 `json:"steps"`
	Workspace   models.WorkspaceSettings `json:"workspace"`
	Email       *models.EmailSettings    `json:"email"`
	Sending     models.SendingSettings   `json:"sending"`
	UsersActive int                      `json:"users_active"`
	UsersMax    int                      `json:"users_max"`
	Tenant      TenantInfo               `json:"tenant"`
}

// TenantInfo é a identidade do ambiente para o front (também em /me/workspace).
type TenantInfo struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	LogoURL  string `json:"logo_url"`
	Version  string `json:"version"`
	BasePath string `json:"base_path"`
	AppURL   string `json:"app_url"`
	// PlanEnabled: há painel configurado para o "Meu plano".
	PlanEnabled bool `json:"plan_enabled"`
	SetupDone   bool `json:"setup_done"`
}

// Tenant monta a identidade do ambiente a partir da configuração e do que o
// cliente gravou (nome, cor e logo em Configurações → Empresa).
func Tenant(db *sql.DB) TenantInfo {
	c := utils.Cfg
	w := models.LoadWorkspaceSettings(db, c.TenantNome, c.CorPrimaria, c.LogoURL)
	st := models.LoadSetupState(db)
	return TenantInfo{Slug: c.TenantSlug, Name: w.Name, Color: w.Color, LogoURL: w.LogoURL, Version: c.Versao,
		BasePath: c.BasePath, AppURL: c.AppURL, PlanEnabled: c.PainelURL != "" && c.TokenInterno != "", SetupDone: st.Completed}
}

func LoadSetupStatus(db *sql.DB) SetupStatus {
	st := models.LoadSetupState(db)
	ativos, _ := models.CountActiveUsers(db)
	var email *models.EmailSettings
	if e := models.LoadEmailSettings(db); e != nil {
		pub := e.Publica()
		email = &pub
	}
	c := utils.Cfg
	return SetupStatus{Completed: st.Completed, Template: st.Template, Steps: st.Steps,
		Workspace: models.LoadWorkspaceSettings(db, c.TenantNome, c.CorPrimaria, c.LogoURL),
		Email:     email, Sending: models.LoadSendingSettings(db), UsersActive: ativos, UsersMax: c.UsuariosMax,
		Tenant: Tenant(db)}
}

// ResultadoTemplate conta o que o modelo criou.
type ResultadoTemplate struct {
	Pipelines    int `json:"pipelines"`
	Etapas       int `json:"etapas"`
	Emails       int `json:"emails"`
	Cadencias    int `json:"cadencias"`
	Propriedades int `json:"propriedades"`
	Snippets     int `json:"snippets"`
	// Ignorados: itens que já existiam com o mesmo nome (aplicar de novo não duplica).
	Ignorados int `json:"ignorados"`
}

// AplicarTemplate semeia o modelo escolhido. É idempotente por nome: rodar de
// novo não duplica pipeline, modelo ou campo que já exista. O pipeline
// "Vendas" criado pela migração inicial continua lá — o cliente apaga se quiser.
func AplicarTemplate(db *sql.DB, codigo string, userID *int64) (ResultadoTemplate, error) {
	var res ResultadoTemplate
	t, ok := TemplatePorCodigo(codigo)
	if !ok {
		return res, fmt.Errorf("modelo %q não existe", codigo)
	}

	existentes, err := models.ListPipelines(db)
	if err != nil {
		return res, err
	}
	nomesPipeline := map[string]bool{}
	for _, p := range existentes {
		nomesPipeline[p.Name] = true
	}
	for i, pm := range t.Pipelines {
		if nomesPipeline[pm.Nome] {
			res.Ignorados++
			continue
		}
		p := &models.Pipeline{Name: pm.Nome, Position: len(existentes) + i + 1}
		if err := models.CreatePipeline(db, p); err != nil {
			return res, fmt.Errorf("criando pipeline %s: %w", pm.Nome, err)
		}
		res.Pipelines++
		for j, e := range pm.Etapas {
			s := &models.PipelineStage{PipelineID: p.ID, Name: e.Nome, Position: j + 1, Probability: e.Probabilidade, IsWon: e.Ganho, IsLost: e.Perda}
			if err := models.CreateStage(db, s); err != nil {
				return res, fmt.Errorf("criando etapa %s: %w", e.Nome, err)
			}
			res.Etapas++
		}
	}

	modelos, _ := models.ListMessageTemplates(db)
	nomesModelo := map[string]int64{}
	for _, m := range modelos {
		nomesModelo[m.Name] = m.ID
	}
	for _, em := range t.Emails {
		if _, ok := nomesModelo[em.Nome]; ok {
			res.Ignorados++
			continue
		}
		m := &models.MessageTemplate{Name: em.Nome, Subject: em.Assunto, Body: em.Corpo, CreatedBy: userID}
		if err := models.CreateMessageTemplate(db, m); err != nil {
			return res, fmt.Errorf("criando modelo %s: %w", em.Nome, err)
		}
		nomesModelo[em.Nome] = m.ID
		res.Emails++
	}

	seqs, _ := models.ListSequences(db)
	nomesSeq := map[string]bool{}
	for _, s := range seqs {
		nomesSeq[s.Name] = true
	}
	for _, cm := range t.Cadencias {
		if nomesSeq[cm.Nome] {
			res.Ignorados++
			continue
		}
		seq := &models.Sequence{Name: cm.Nome, Description: cm.Descricao, Active: true, ExitOnReply: true, ExitOnMeeting: true, CreatedBy: userID, OwnerID: userID}
		for _, p := range cm.Passos {
			seq.Steps = append(seq.Steps, models.SequenceStep{Kind: p.Tipo, DelayDays: p.DiasDepois, Subject: p.Assunto, Body: p.Corpo, Title: p.Titulo, Note: p.Nota})
		}
		if err := models.ValidateSequence(seq); err != nil {
			return res, fmt.Errorf("cadência %s inválida: %w", cm.Nome, err)
		}
		if err := models.CreateSequence(db, seq); err != nil {
			return res, fmt.Errorf("criando cadência %s: %w", cm.Nome, err)
		}
		res.Cadencias++
	}

	for _, pm := range t.Propriedades {
		props, _ := models.ListProperties(db, pm.Entidade)
		chave := models.SlugifyKey(pm.Rotulo)
		existe := false
		for _, p := range props {
			if p.Key == chave {
				existe = true
				break
			}
		}
		if existe {
			res.Ignorados++
			continue
		}
		p := &models.CustomProperty{Entity: pm.Entidade, Key: chave, Label: pm.Rotulo, FieldType: pm.Tipo, GroupName: pm.Grupo, CreatedBy: userID, Position: len(props) + 1}
		for _, o := range pm.Opcoes {
			p.Options = append(p.Options, models.PropertyOption{Value: models.SlugifyKey(o), Label: o})
		}
		if err := models.ValidateProperty(p); err != nil {
			return res, fmt.Errorf("campo %s inválido: %w", pm.Rotulo, err)
		}
		if err := models.CreateProperty(db, p); err != nil {
			return res, fmt.Errorf("criando campo %s: %w", pm.Rotulo, err)
		}
		res.Propriedades++
	}

	snips, _ := models.ListSnippets(db)
	nomesSnip := map[string]bool{}
	for _, s := range snips {
		nomesSnip[s.Name] = true
	}
	for _, sm := range t.Snippets {
		if nomesSnip[sm.Nome] {
			res.Ignorados++
			continue
		}
		s := &models.Snippet{Name: sm.Nome, Shortcut: sm.Atalho, Body: sm.Corpo, CreatedBy: userID}
		if err := models.CreateSnippet(db, s); err != nil {
			return res, fmt.Errorf("criando snippet %s: %w", sm.Nome, err)
		}
		res.Snippets++
	}

	st := models.LoadSetupState(db)
	st.Template = codigo
	st.MarcarPasso("template")
	if err := models.SetSetting(db, models.SettingSetup, st, userID); err != nil {
		return res, err
	}
	log.Printf("setup: modelo %q aplicado (%d pipelines, %d modelos, %d cadências, %d campos)", codigo, res.Pipelines, res.Emails, res.Cadencias, res.Propriedades)
	return res, nil
}

// MarcarPassoSetup registra um passo concluído (empresa, email, equipe…).
func MarcarPassoSetup(db *sql.DB, passo string, userID *int64) error {
	st := models.LoadSetupState(db)
	st.MarcarPasso(passo)
	return models.SetSetting(db, models.SettingSetup, st, userID)
}

// ConcluirSetup fecha o assistente; a partir daí o CRM abre direto no dashboard.
func ConcluirSetup(db *sql.DB, userID *int64) error {
	st := models.LoadSetupState(db)
	st.Completed = true
	agora := time.Now()
	st.CompletedAt = &agora
	st.MarcarPasso("concluido")
	return models.SetSetting(db, models.SettingSetup, st, userID)
}
