package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services/enriquecimento"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// -----------------------------------------------------------------------------
// Inteligência de empresas: consulta pública de CNPJ, enriquecimento por
// Receita Federal / BigDataCorp / Serasa e as configurações das fontes
// (docs/FUNIL-E-INTELIGENCIA.md, Parte 2).
// -----------------------------------------------------------------------------

// cnpjBancoDoBrasil é o CNPJ público usado no teste de conexão das fontes.
const cnpjBancoDoBrasil = "00000000000191"

func registroEnriquecimento() (*enriquecimento.Registro, models.EnrichmentSettings) {
	cfg := models.LoadEnrichmentSettings(utils.DB)
	return enriquecimento.NovoRegistro(cfg), cfg
}

func unprocessable(ctx iris.Context, msg string) {
	ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"error": msg})
}

func badGateway(ctx iris.Context, msg string) {
	ctx.StopWithJSON(iris.StatusBadGateway, iris.Map{"error": msg})
}

// responderFalhaExterna traduz uma Falha da fonte em status HTTP.
func responderFalhaExterna(ctx iris.Context, err error) {
	msg := enriquecimento.MensagemDaFalha(err)
	switch enriquecimento.TipoDaFalha(err) {
	case enriquecimento.FalhaSemResultado:
		ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": msg})
	case enriquecimento.FalhaLimite:
		ctx.StopWithJSON(iris.StatusTooManyRequests, iris.Map{"error": msg})
	case enriquecimento.FalhaEntrada, enriquecimento.FalhaConfig:
		unprocessable(ctx, msg)
	default:
		badGateway(ctx, msg)
	}
}

// usoDoMes devolve o consumo do mês com o limite configurado.
func usoDoMes(cfg models.EnrichmentSettings) models.UsoMes {
	uso, err := models.EnrichmentUsage(utils.DB, time.Now())
	if err != nil {
		uso = models.UsoMes{Mes: time.Now().Format("2006-01"), PorProvedor: map[string]models.UsoProvedor{}}
	}
	uso.LimiteCentavos = cfg.LimiteMensalCentavos
	return uso
}

// consultaResultado é o que cada fonte devolve numa prévia.
type consultaResultado struct {
	Provedor      string `json:"provedor"`
	Status        string `json:"status"`
	Mensagem      string `json:"mensagem"`
	Demo          bool   `json:"demo"`
	CustoCentavos int    `json:"custo_centavos"`
	Cache         bool   `json:"cache"`
	EnrichmentID  int64  `json:"enrichment_id,omitempty"`
	perfil        *enriquecimento.Perfil
}

// executarConsulta roda a fonte para a empresa, grava a linha em
// company_enrichments (aplicado_em NULL) e devolve o resultado. A Receita
// reaproveita a última consulta ok com menos de prazo_cache_dias, salvo forçar.
func executarConsulta(ctx context.Context, reg *enriquecimento.Registro, company *models.Company,
	provedor string, opts enriquecimento.Opcoes, userID *int64) consultaResultado {
	res := consultaResultado{Provedor: provedor, Status: models.EnrichmentErro}
	cnpj := enriquecimento.SomenteDigitos(company.CNPJ)

	if provedor == models.ProvedorReceita && !opts.Forcar {
		prazo := time.Duration(reg.Cfg.PrazoCacheDias) * 24 * time.Hour
		if row, err := models.LastOkEnrichment(utils.DB, company.ID, provedor, prazo); err == nil && row != nil {
			if p := enriquecimento.PerfilDeJSON(row.Perfil); p != nil && enriquecimento.SomenteDigitos(p.Texto("cnpj")) == cnpj {
				res.Status, res.Cache, res.EnrichmentID, res.perfil = models.EnrichmentOK, true, row.ID, p
				return res
			}
		}
	}

	prov, err := reg.Provedor(provedor)
	if err != nil {
		res.Mensagem = enriquecimento.MensagemDaFalha(err)
		return res
	}
	datasets := ""
	if provedor == models.ProvedorBigDataCorp {
		ds := opts.Datasets
		if len(ds) == 0 {
			ds = reg.Cfg.BigDataCorp.DatasetsPadrao
		}
		datasets = strings.Join(ds, ",")
	}

	perfil, bruto, err := prov.Consultar(ctx, cnpj, opts)
	row := models.Enrichment{CompanyID: company.ID, Provedor: provedor, Datasets: datasets, UserID: userID}
	switch {
	case err != nil:
		res.Mensagem = enriquecimento.MensagemDaFalha(err)
		if enriquecimento.TipoDaFalha(err) == enriquecimento.FalhaSemResultado {
			res.Status = models.EnrichmentSemResultado
		}
		row.Status, row.Mensagem = res.Status, res.Mensagem
	default:
		res.Status = models.EnrichmentOK
		res.Demo = perfil.Demo
		res.Mensagem = perfil.Mensagem
		res.perfil = perfil
		if bruto != nil {
			res.Cache = bruto.Cache
			if b, err := json.Marshal(bruto); err == nil {
				row.Bruto = b
			}
		}
		if !res.Cache {
			res.CustoCentavos = perfil.CustoCentavos
		}
		row.Status, row.Mensagem, row.Demo, row.CustoCentavos = res.Status, res.Mensagem, perfil.Demo, res.CustoCentavos
		if b, err := json.Marshal(perfil); err == nil {
			row.Perfil = b
		}
	}
	if err := models.InsertEnrichment(utils.DB, &row); err != nil {
		res.Status = models.EnrichmentErro
		res.Mensagem = "não foi possível gravar a consulta"
		res.perfil = nil
		return res
	}
	res.EnrichmentID = row.ID
	return res
}

// perfilConsolidado mescla a última consulta ok de cada fonte.
func perfilConsolidado(companyID int64) (*enriquecimento.Perfil, []models.Enrichment) {
	rows, err := models.LastOkEnrichmentsPerProvider(utils.DB, companyID)
	if err != nil {
		return nil, nil
	}
	perfis := make([]*enriquecimento.Perfil, 0, len(rows))
	for _, r := range rows {
		if p := enriquecimento.PerfilDeJSON(r.Perfil); p != nil {
			perfis = append(perfis, p)
		}
	}
	return enriquecimento.Mesclar(perfis...), rows
}

// gravarIntel atualiza companies.intel com o perfil consolidado.
func gravarIntel(companyID int64) {
	consolidado, _ := perfilConsolidado(companyID)
	if consolidado == nil {
		return
	}
	if b, err := json.Marshal(consolidado); err == nil {
		_ = models.SetCompanyIntel(utils.DB, companyID, b)
	}
}

// fraseEnriquecimento monta "Dados enriquecidos pela Receita Federal: 7 campos".
func fraseEnriquecimento(provedores []string, n int) string {
	nomes := make([]string, 0, len(provedores))
	for _, p := range provedores {
		nomes = append(nomes, models.NomeProvedor(p))
	}
	campos := "campos"
	if n == 1 {
		campos = "campo"
	}
	if len(nomes) == 1 {
		prep := "por"
		switch provedores[0] {
		case models.ProvedorReceita, models.ProvedorSerasa:
			prep = "pela"
		case models.ProvedorBigDataCorp:
			prep = "pelo"
		}
		return fmt.Sprintf("Dados enriquecidos %s %s: %d %s", prep, nomes[0], n, campos)
	}
	return fmt.Sprintf("Dados enriquecidos por %s: %d %s", strings.Join(nomes, " e "), n, campos)
}

// aplicarCampos grava os campos escolhidos, as fontes e marca as consultas.
// Devolve os campos efetivamente aplicados e os provedores envolvidos.
func aplicarCampos(company *models.Company, diff []enriquecimento.CampoDiff, escolhidos map[string]bool,
	idPorProvedor map[string]int64, ids []int64) ([]string, []string, error) {
	valores := map[string]any{}
	fontes := []models.FieldSource{}
	aplicados := []string{}
	provedores := []string{}
	vistos := map[string]bool{}
	for _, d := range diff {
		if escolhidos != nil && !escolhidos[d.Campo] {
			continue
		}
		if !models.EnrichableColumns[d.Campo] {
			continue
		}
		valores[d.Campo] = d.Proposto
		aplicados = append(aplicados, d.Campo)
		fs := models.FieldSource{Campo: d.Campo, Provedor: d.Provedor}
		if id, ok := idPorProvedor[d.Provedor]; ok {
			fs.EnrichmentID = &id
		}
		fontes = append(fontes, fs)
		if !vistos[d.Provedor] {
			vistos[d.Provedor] = true
			provedores = append(provedores, d.Provedor)
		}
	}
	if len(aplicados) > 0 {
		if err := models.ApplyEnrichedFields(utils.DB, company.ID, valores); err != nil {
			return nil, nil, err
		}
		if err := models.UpsertFieldSources(utils.DB, company.ID, fontes); err != nil {
			return nil, nil, err
		}
	}
	gravarIntel(company.ID)
	if err := models.MarkEnrichmentsApplied(utils.DB, ids, aplicados); err != nil {
		return nil, nil, err
	}
	return aplicados, provedores, nil
}

// -----------------------------------------------------------------------------
// GET /cnpj/{cnpj}
// -----------------------------------------------------------------------------

// LookupCNPJ consulta a Receita (grátis) para pré-preencher o formulário.
func LookupCNPJ(ctx iris.Context) {
	cnpj := enriquecimento.SomenteDigitos(ctx.Params().GetString("cnpj"))
	if !enriquecimento.ValidarCNPJ(cnpj) {
		unprocessable(ctx, "CNPJ inválido")
		return
	}
	reg, _ := registroEnriquecimento()
	prov, err := reg.Provedor(models.ProvedorReceita)
	if err != nil {
		responderFalhaExterna(ctx, err)
		return
	}
	perfil, _, err := prov.Consultar(ctx.Request().Context(), cnpj, enriquecimento.Opcoes{
		Forcar: ctx.URLParamBoolDefault("forcar", false),
	})
	if err != nil {
		responderFalhaExterna(ctx, err)
		return
	}
	var existente iris.Map
	if c, err := models.CompanyByCNPJ(utils.DB, cnpj); err == nil {
		existente = iris.Map{"id": c.ID, "name": c.Name}
	}
	ctx.JSON(iris.Map{"perfil": perfil, "existente": existente})
}

// -----------------------------------------------------------------------------
// GET/DELETE /companies/{id}/intel
// -----------------------------------------------------------------------------

// CompanyIntel devolve a ficha com o perfil consolidado, fontes e histórico.
func CompanyIntel(ctx iris.Context) {
	company, err := models.CompanyByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	_, cfg := registroEnriquecimento()
	intel, _ := perfilConsolidado(company.ID)
	fontes, err := models.FieldSourcesMap(utils.DB, company.ID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	historico, err := models.ListEnrichmentHistory(utils.DB, company.ID, 50)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{
		"company":   company,
		"intel":     intel,
		"fontes":    fontes,
		"historico": historico,
		"uso_mes":   usoDoMes(cfg),
	})
}

// DeleteCompanyIntel remove intel, fontes e histórico; os campos aplicados ficam.
func DeleteCompanyIntel(ctx iris.Context) {
	company, err := models.CompanyByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeleteCompanyIntel(utils.DB, company.ID); err != nil {
		serverError(ctx, err)
		return
	}
	logActivity(ctx, models.Activity{Kind: models.ActivitySistema, Content: "Dados de inteligência removidos", CompanyID: &company.ID})
	audit(ctx, models.AuditDelete, "empresa", company.ID, "removeu os dados de inteligência da empresa "+company.Name)
	ctx.JSON(iris.Map{"message": "dados de inteligência removidos"})
}

// -----------------------------------------------------------------------------
// POST /companies/{id}/enrich/preview
// -----------------------------------------------------------------------------

type previewRequest struct {
	Provedores []string `json:"provedores"`
	Datasets   []string `json:"datasets"`
	Forcar     bool     `json:"forcar"`
}

func normalizarProvedores(lista []string) ([]string, string) {
	out := []string{}
	visto := map[string]bool{}
	for _, p := range lista {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || visto[p] {
			continue
		}
		if !enriquecimento.CodigoValido(p) {
			return nil, "fonte desconhecida: " + p
		}
		visto[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		out = append(out, models.ProvedorReceita)
	}
	return out, ""
}

func normalizarDatasets(lista []string) ([]string, string) {
	out := []string{}
	visto := map[string]bool{}
	for _, d := range lista {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || visto[d] {
			continue
		}
		if !models.BigDataCorpDatasetValido(d) {
			return nil, "conjunto de dados desconhecido: " + d
		}
		visto[d] = true
		out = append(out, d)
	}
	return out, ""
}

// EnrichPreview executa as consultas, grava o histórico e devolve o diff.
func EnrichPreview(ctx iris.Context) {
	company, err := models.CompanyByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if !enriquecimento.ValidarCNPJ(company.CNPJ) {
		unprocessable(ctx, "Informe um CNPJ válido para enriquecer a empresa")
		return
	}
	var req previewRequest
	if err := ctx.ReadJSON(&req); err != nil && err.Error() != "EOF" {
		badRequest(ctx, "dados inválidos")
		return
	}
	provedores, msg := normalizarProvedores(req.Provedores)
	if msg != "" {
		badRequest(ctx, msg)
		return
	}
	datasets, msg := normalizarDatasets(req.Datasets)
	if msg != "" {
		badRequest(ctx, msg)
		return
	}
	reg, cfg := registroEnriquecimento()

	// Fontes pagas precisam estar configuradas (ou em demonstração) e caber no mês.
	estimado := 0
	for _, p := range provedores {
		if !enriquecimento.Pago(p) {
			continue
		}
		if _, err := reg.Provedor(p); err != nil {
			unprocessable(ctx, enriquecimento.MensagemDaFalha(err))
			return
		}
		estimado += reg.CustoEstimado(p, datasets)
	}
	if estimado > 0 && cfg.LimiteMensalCentavos > 0 {
		uso := usoDoMes(cfg)
		if uso.CustoCentavos+estimado > cfg.LimiteMensalCentavos {
			ctx.StopWithJSON(iris.StatusTooManyRequests, iris.Map{
				"error": fmt.Sprintf("Limite mensal de consultas pagas atingido (R$ %.2f de R$ %.2f). Ajuste em Configurações → Integrações.",
					float64(uso.CustoCentavos)/100, float64(cfg.LimiteMensalCentavos)/100),
				"uso_mes": uso,
			})
			return
		}
	}

	uid := userIDPtr(ctx)
	opts := enriquecimento.Opcoes{Datasets: datasets, Forcar: req.Forcar}
	consultas := []consultaResultado{}
	ids := []int64{}
	perfis := []*enriquecimento.Perfil{}
	custo := 0
	for _, p := range provedores {
		res := executarConsulta(ctx.Request().Context(), reg, company, p, opts, uid)
		consultas = append(consultas, res)
		if res.EnrichmentID > 0 {
			ids = append(ids, res.EnrichmentID)
		}
		if res.perfil != nil {
			perfis = append(perfis, res.perfil)
		}
		custo += res.CustoCentavos
	}
	mesclado := enriquecimento.Mesclar(perfis...)
	ctx.JSON(iris.Map{
		"enrichment_ids": ids,
		"consultas":      consultas,
		"perfil":         mesclado,
		"diff":           enriquecimento.Diff(company, mesclado, cfg.Sobrescrever),
		"custo_centavos": custo,
	})
}

// -----------------------------------------------------------------------------
// POST /companies/{id}/enrich/apply
// -----------------------------------------------------------------------------

type applyRequest struct {
	EnrichmentIDs []int64  `json:"enrichment_ids"`
	Campos        []string `json:"campos"`
}

// EnrichApply grava os campos escolhidos a partir das consultas da prévia.
func EnrichApply(ctx iris.Context) {
	company, err := models.CompanyByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	var req applyRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	ids := []int64{}
	visto := map[int64]bool{}
	for _, id := range req.EnrichmentIDs {
		if id > 0 && !visto[id] {
			visto[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		badRequest(ctx, "informe as consultas da prévia (enrichment_ids)")
		return
	}
	rows, err := models.EnrichmentsByIDs(utils.DB, company.ID, ids)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if len(rows) != len(ids) {
		unprocessable(ctx, "alguma consulta informada não pertence a esta empresa")
		return
	}
	perfis := []*enriquecimento.Perfil{}
	idPorProvedor := map[string]int64{}
	for _, r := range rows {
		if r.Status != models.EnrichmentOK {
			continue
		}
		if p := enriquecimento.PerfilDeJSON(r.Perfil); p != nil {
			perfis = append(perfis, p)
			idPorProvedor[r.Provedor] = r.ID
		}
	}
	mesclado := enriquecimento.Mesclar(perfis...)
	if mesclado == nil {
		unprocessable(ctx, "nenhuma das consultas trouxe resultado para aplicar")
		return
	}
	escolhidos := map[string]bool{}
	for _, c := range req.Campos {
		c = strings.ToLower(strings.TrimSpace(c))
		if enriquecimento.CampoConhecido(c) {
			escolhidos[c] = true
		}
	}
	if len(escolhidos) == 0 {
		badRequest(ctx, "selecione ao menos um campo para aplicar")
		return
	}
	// "sempre": o diff recalculado traz tudo que difere; o usuário já escolheu.
	diff := enriquecimento.Diff(company, mesclado, "sempre")
	aplicados, provedores, err := aplicarCampos(company, diff, escolhidos, idPorProvedor, ids)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if len(aplicados) > 0 {
		logActivity(ctx, models.Activity{
			Kind:      models.ActivitySistema,
			Content:   fraseEnriquecimento(provedores, len(aplicados)),
			CompanyID: &company.ID,
		})
		labels := make([]string, 0, len(aplicados))
		for _, c := range aplicados {
			labels = append(labels, enriquecimento.LabelCampo(c))
		}
		audit(ctx, models.AuditUpdate, "empresa", company.ID,
			fmt.Sprintf("enriqueceu a empresa %s (%d campos: %s)", company.Name, len(aplicados), strings.Join(labels, ", ")))
	}
	atualizada, err := models.CompanyByID(utils.DB, company.ID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"company": atualizada, "aplicados": aplicados})
}

// -----------------------------------------------------------------------------
// POST /companies/enrich/bulk
// -----------------------------------------------------------------------------

type bulkRequest struct {
	IDs        []int64  `json:"ids"`
	Provedores []string `json:"provedores"`
}

// EnrichBulk enriquece várias empresas pela Receita (grátis), aplicando
// automaticamente com a regra "sobrescrever".
func EnrichBulk(ctx iris.Context) {
	var req bulkRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if len(req.IDs) == 0 {
		badRequest(ctx, "selecione ao menos uma empresa")
		return
	}
	if len(req.IDs) > 100 {
		badRequest(ctx, "no máximo 100 empresas por vez")
		return
	}
	provedores, msg := normalizarProvedores(req.Provedores)
	if msg != "" {
		badRequest(ctx, msg)
		return
	}
	for _, p := range provedores {
		if p != models.ProvedorReceita {
			badRequest(ctx, "em massa só a Receita Federal (grátis) está disponível")
			return
		}
	}
	reg, cfg := registroEnriquecimento()
	uid := userIDPtr(ctx)

	type erroItem struct {
		ID       int64  `json:"id"`
		Mensagem string `json:"mensagem"`
	}
	erros := []erroItem{}
	processados, enriquecidos, semCNPJ := 0, 0, 0
	vistos := map[int64]bool{}
	for _, id := range req.IDs {
		if id <= 0 || vistos[id] {
			continue
		}
		vistos[id] = true
		processados++
		company, err := models.CompanyByID(utils.DB, id)
		if err != nil {
			erros = append(erros, erroItem{ID: id, Mensagem: "empresa não encontrada"})
			continue
		}
		if !enriquecimento.ValidarCNPJ(company.CNPJ) {
			semCNPJ++
			continue
		}
		res := executarConsulta(ctx.Request().Context(), reg, company, models.ProvedorReceita, enriquecimento.Opcoes{}, uid)
		if res.Status != models.EnrichmentOK || res.perfil == nil {
			erros = append(erros, erroItem{ID: id, Mensagem: res.Mensagem})
			continue
		}
		diff := enriquecimento.Diff(company, res.perfil, cfg.Sobrescrever)
		sugeridos := map[string]bool{}
		for _, d := range diff {
			if d.Sugerido {
				sugeridos[d.Campo] = true
			}
		}
		aplicados, provs, err := aplicarCampos(company, diff, sugeridos,
			map[string]int64{models.ProvedorReceita: res.EnrichmentID}, []int64{res.EnrichmentID})
		if err != nil {
			erros = append(erros, erroItem{ID: id, Mensagem: "não foi possível gravar os dados"})
			continue
		}
		if len(aplicados) > 0 {
			enriquecidos++
			logActivity(ctx, models.Activity{
				Kind: models.ActivitySistema, Content: fraseEnriquecimento(provs, len(aplicados)), CompanyID: &company.ID,
			})
		}
	}
	audit(ctx, models.AuditBulk, "empresa", 0,
		fmt.Sprintf("enriqueceu %d de %d empresas pela Receita Federal", enriquecidos, processados))
	ctx.JSON(iris.Map{
		"processados":  processados,
		"enriquecidos": enriquecidos,
		"sem_cnpj":     semCNPJ,
		"erros":        erros,
	})
}

// -----------------------------------------------------------------------------
// Configurações → Integrações
// -----------------------------------------------------------------------------

// GetEnrichmentSettings devolve a configuração (sem segredos), o catálogo de
// fontes e o uso do mês.
func GetEnrichmentSettings(ctx iris.Context) {
	reg, cfg := registroEnriquecimento()
	ctx.JSON(iris.Map{
		"settings": cfg.Publica(),
		"fontes":   reg.Catalogo(),
		"uso_mes":  usoDoMes(cfg),
	})
}

// UpdateEnrichmentSettings grava a configuração; segredos vazios mantêm os atuais.
func UpdateEnrichmentSettings(ctx iris.Context) {
	var cfg models.EnrichmentSettings
	if err := ctx.ReadJSON(&cfg); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if err := cfg.Validate(); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	atual := models.LoadEnrichmentSettings(utils.DB)

	if tok := strings.TrimSpace(cfg.BigDataCorp.AccessToken); tok != "" {
		cifrado, err := utils.Cifrar(tok)
		if err != nil {
			serverError(ctx, err)
			return
		}
		cfg.BigDataCorp.AccessTokenCifrado = cifrado
	} else {
		cfg.BigDataCorp.AccessTokenCifrado = atual.BigDataCorp.AccessTokenCifrado
	}
	if sec := strings.TrimSpace(cfg.Serasa.ClientSecret); sec != "" {
		cifrado, err := utils.Cifrar(sec)
		if err != nil {
			serverError(ctx, err)
			return
		}
		cfg.Serasa.ClientSecretCifrado = cifrado
	} else {
		cfg.Serasa.ClientSecretCifrado = atual.Serasa.ClientSecretCifrado
	}
	if cfg.BigDataCorp.Habilitado && !cfg.BigDataCorp.Demo && (cfg.BigDataCorp.TokenID == "" || cfg.BigDataCorp.AccessTokenCifrado == "") {
		badRequest(ctx, "informe Token ID e Access Token do BigDataCorp (ou ative o modo demonstração)")
		return
	}
	if cfg.Serasa.Habilitado && !cfg.Serasa.Demo && (cfg.Serasa.ClientID == "" || cfg.Serasa.ClientSecretCifrado == "") {
		badRequest(ctx, "informe client_id e client_secret do Serasa (ou ative o modo demonstração)")
		return
	}
	gravar := cfg.ParaGravar()
	uid := userIDPtr(ctx)
	if err := models.SetSetting(utils.DB, models.SettingEnrichment, gravar, uid); err != nil {
		serverError(ctx, err)
		return
	}
	// Tokens Serasa em cache valem para as credenciais antigas.
	utils.Cache.DelPrefixo("enriq:serasa:token:")
	audit(ctx, models.AuditUpdate, "configuracao", 0, "integrações de dados atualizadas")
	ctx.JSON(iris.Map{"settings": gravar.Publica()})
}

// TestEnrichmentProvider faz uma consulta mínima para validar a configuração.
func TestEnrichmentProvider(ctx iris.Context) {
	var req struct {
		Provedor string `json:"provedor"`
		CNPJ     string `json:"cnpj"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	provedor := strings.ToLower(strings.TrimSpace(req.Provedor))
	if !enriquecimento.CodigoValido(provedor) {
		badRequest(ctx, "fonte desconhecida")
		return
	}
	cnpj := enriquecimento.SomenteDigitos(req.CNPJ)
	if cnpj == "" {
		cnpj = cnpjBancoDoBrasil
	}
	if !enriquecimento.ValidarCNPJ(cnpj) {
		unprocessable(ctx, "CNPJ inválido")
		return
	}
	reg, _ := registroEnriquecimento()
	prov, err := reg.Provedor(provedor)
	if err != nil {
		unprocessable(ctx, enriquecimento.MensagemDaFalha(err))
		return
	}
	resposta := func(ok bool, msg string, custo int, demo bool) {
		ctx.JSON(iris.Map{"ok": ok, "mensagem": msg, "custo_centavos": custo, "demo": demo})
	}
	reqCtx, cancelar := context.WithTimeout(ctx.Request().Context(), 25*time.Second)
	defer cancelar()

	// Serasa real: só o login, que não tem custo.
	if s, ok := prov.(*enriquecimento.Serasa); ok {
		if _, err := s.Login(reqCtx); err != nil {
			resposta(false, enriquecimento.MensagemDaFalha(err), 0, false)
			return
		}
		resposta(true, fmt.Sprintf("Login no Serasa Experian (%s) realizado com sucesso", s.Ambiente), 0, false)
		return
	}
	opts := enriquecimento.Opcoes{Forcar: true}
	if provedor == models.ProvedorBigDataCorp {
		opts.Datasets = []string{"basic_data"}
	}
	perfil, _, err := prov.Consultar(reqCtx, cnpj, opts)
	if err != nil {
		resposta(false, enriquecimento.MensagemDaFalha(err), 0, false)
		return
	}
	nome := perfil.Texto("legal_name")
	if nome == "" {
		nome = enriquecimento.FormatarCNPJ(cnpj)
	}
	msg := fmt.Sprintf("Consulta a %s realizada com sucesso: %s", models.NomeProvedor(provedor), nome)
	if perfil.Demo {
		msg = "Modo demonstração ativo: " + models.NomeProvedor(provedor) + " devolve dados fictícios"
	}
	resposta(true, msg, perfil.CustoCentavos, perfil.Demo)
}

// EnrichmentUsage devolve o consumo dos últimos meses por fonte.
func EnrichmentUsage(ctx iris.Context) {
	_, cfg := registroEnriquecimento()
	meses, err := models.EnrichmentUsageMeses(utils.DB, ctx.URLParamIntDefault("meses", 6))
	if err != nil && err != sql.ErrNoRows {
		serverError(ctx, err)
		return
	}
	for i := range meses {
		meses[i].LimiteCentavos = cfg.LimiteMensalCentavos
	}
	ctx.JSON(iris.Map{"meses": meses})
}
