package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/armando-couto/crm-ia/app/models"
)

// -----------------------------------------------------------------------------
// Job "gerar_lista" (docs/PROSPECCAO.md seção 6.3, com o adendo).
//
// Fases, uma fatia curta por vez, sempre retomáveis pelo estado gravado:
//
//   1. inicio      lê a cota na plataforma e limita a quantidade;
//   2. candidatos  pede candidatos em páginas e escolhe os CNPJs, pulando
//                  bloqueado, já no CRM, já em outra lista e repetido, até
//                  juntar a quantidade, esgotar a busca ou bater o teto de
//                  leitura (20 vezes a quantidade). Origem "cnpjs" não busca:
//                  os candidatos são os CNPJs informados;
//   3. extracao    GRAVA NO ESTADO a lista final já fatiada em lotes de 200 e
//                  só então chama a extração, um lote por fatia, com o
//                  pedido_id da lista. A retomada reenvia exatamente os mesmos
//                  lotes (a plataforma não desconta de novo) e nunca refaz a
//                  busca de candidatos;
//   4. fim         lista "pronta", auditoria "lista gerada".
//
// Cota esgotada no meio encerra o job como concluído ("cota esgotada: N de M
// entregues"). Job cancelado ou falho com itens gravados deixa a lista
// "pronta" (parcial); sem nenhum item, "falhou" (Encerrar).
// -----------------------------------------------------------------------------

func init() { RegistrarProcessadorJob(models.JobGerarLista, processadorGerarLista{}) }

const (
	// GerarListaLote é o tamanho do lote de extração.
	GerarListaLote = 200
	// gerarListaPagina é o teto de candidatos por chamada.
	gerarListaPagina = 5000
	// gerarListaTetoLeitura: lê no máximo 20 candidatos para cada empresa pedida.
	gerarListaTetoLeitura = 20
	// gerarListaMaxRefazer: quantas vezes a busca recomeça por troca de base.
	gerarListaMaxRefazer = 3

	faseGerarCandidatos = "candidatos"
	faseGerarExtracao   = "extracao"
)

// ResultadoGerarLista é o resumo final do job (chave "resultado" do Job).
type ResultadoGerarLista struct {
	Pedidos          int  `json:"pedidos"`
	Entregues        int  `json:"entregues"`
	PuladosNoCRM     int  `json:"pulados_no_crm"`
	PuladosEmListas  int  `json:"pulados_em_listas"`
	PuladosBloqueio  int  `json:"pulados_bloqueio"`
	NaoEncontrados   int  `json:"nao_encontrados"`
	PuladosPF        int  `json:"pulados_pf"`
	CandidatosLidos  int  `json:"candidatos_lidos"`
	LimitadoPelaCota bool `json:"limitado_pela_cota"`
}

// estadoGerarLista é o cursor de retomada do job.
type estadoGerarLista struct {
	Fase       string              `json:"fase"`
	Quantidade int                 `json:"quantidade"` // alvo depois de limitar à cota
	Cursor     string              `json:"cursor,omitempty"`
	Versao     string              `json:"versao,omitempty"`
	Refeitas   int                 `json:"refeitas,omitempty"`
	Escolhidos []string            `json:"escolhidos,omitempty"` // só na fase de candidatos
	Lotes      [][]string          `json:"lotes,omitempty"`      // gravado antes da primeira extração
	Lote       int                 `json:"lote"`                 // próximo lote a extrair
	Res        ResultadoGerarLista `json:"res"`
}

type processadorGerarLista struct{}

func (processadorGerarLista) Fatia(ctx context.Context, e *ExecucaoJob) (Fatia, error) {
	if e.Job.ListaID == nil {
		return Fatia{}, errors.New("a lista deste processamento foi excluída")
	}
	lista, err := models.ProspListaByID(e.DB, *e.Job.ListaID)
	if err == sql.ErrNoRows {
		return Fatia{}, errors.New("a lista deste processamento foi excluída")
	}
	if err != nil {
		return Fatia{}, Transitorio(err)
	}
	var est estadoGerarLista
	if err := e.Estado(&est); err != nil {
		return Fatia{}, err
	}
	switch est.Fase {
	case "":
		return gerarListaInicio(ctx, e, lista, &est)
	case faseGerarCandidatos:
		return gerarListaCandidatos(ctx, e, lista, &est)
	case faseGerarExtracao:
		return gerarListaExtracao(ctx, e, lista, &est)
	}
	return Fatia{}, fmt.Errorf("estado desconhecido do processamento: %q", est.Fase)
}

// gerarListaInicio lê a cota, limita a quantidade e prepara a fase seguinte.
func gerarListaInicio(ctx context.Context, e *ExecucaoJob, lista *models.ProspLista, est *estadoGerarLista) (Fatia, error) {
	resumo, err := PlataformaResumo(ctx)
	if err != nil {
		return Fatia{}, err
	}
	if !resumo.Habilitada {
		motivo := resumo.Motivo
		if motivo == "" {
			motivo = "a prospecção não está disponível neste ambiente"
		}
		return Fatia{}, errors.New(motivo)
	}
	if resumo.Cota.Restante <= 0 {
		return Fatia{}, errors.New("cota mensal de prospecção esgotada")
	}
	est.Res.Pedidos = lista.QuantidadePedida
	est.Quantidade = lista.QuantidadePedida
	if resumo.Cota.Restante < est.Quantidade {
		est.Quantidade = resumo.Cota.Restante
		est.Res.LimitadoPelaCota = true
	}
	est.Versao = resumo.Base.Versao
	if err := e.DefinirTotal(est.Quantidade); err != nil {
		return Fatia{}, Transitorio(err)
	}

	if lista.Origem == models.ListaOrigemCNPJs {
		// Lista específica: não há busca. As exclusões valem do mesmo jeito.
		escolhidos, err := gerarListaFiltrar(e.DB, lista, est, lista.Filtros.CNPJs)
		if err != nil {
			return Fatia{}, Transitorio(err)
		}
		est.Res.CandidatosLidos = len(lista.Filtros.CNPJs)
		est.Escolhidos = escolhidos
		return gerarListaFecharCandidatos(e, lista, est)
	}
	est.Fase = faseGerarCandidatos
	if err := e.SalvarEstado(est); err != nil {
		return Fatia{}, Transitorio(err)
	}
	return Fatia{Mensagem: "buscando empresas na base"}, nil
}

// gerarListaFiltrar aplica as exclusões a uma página de candidatos e devolve
// os que entram, na ordem recebida, sem passar da quantidade.
func gerarListaFiltrar(db *sql.DB, lista *models.ProspLista, est *estadoGerarLista, cnpjs []string) ([]string, error) {
	escolhidos := est.Escolhidos
	if len(cnpjs) == 0 {
		return escolhidos, nil
	}
	bloqueados, err := models.CNPJsBloqueados(db, cnpjs)
	if err != nil {
		return nil, err
	}
	noCRM := map[string]int64{}
	if lista.ExcluirNoCRM {
		if noCRM, err = models.CNPJsNoCRM(db, cnpjs); err != nil {
			return nil, err
		}
	}
	emListas := map[string]bool{}
	if lista.ExcluirEmListas {
		if emListas, err = models.CNPJsEmListas(db, cnpjs, lista.ID); err != nil {
			return nil, err
		}
	}
	vistos := make(map[string]bool, len(escolhidos)+len(cnpjs))
	for _, c := range escolhidos {
		vistos[c] = true
	}
	for _, c := range cnpjs {
		c = models.NormalizarCNPJ(c)
		if c == "" || vistos[c] {
			continue
		}
		vistos[c] = true
		switch {
		case bloqueados[c]:
			est.Res.PuladosBloqueio++
		case noCRM[c] > 0:
			est.Res.PuladosNoCRM++
		case emListas[c]:
			est.Res.PuladosEmListas++
		case len(escolhidos) < est.Quantidade:
			escolhidos = append(escolhidos, c)
		}
	}
	return escolhidos, nil
}

// gerarListaCandidatos lê uma página de candidatos por fatia.
func gerarListaCandidatos(ctx context.Context, e *ExecucaoJob, lista *models.ProspLista, est *estadoGerarLista) (Fatia, error) {
	falta := est.Quantidade - len(est.Escolhidos)
	limite := min(gerarListaPagina, max(falta*2, 500))
	pagina, err := PlataformaCandidatos(ctx, CandidatosPedido{
		PedidoID: lista.PedidoID, Filtros: lista.Filtros, Ordem: lista.Ordem, Limite: limite, Cursor: est.Cursor,
	})
	if err != nil {
		if p, ok := ComoErroPlataforma(err); ok && p.Codigo == CodigoBaseAtualizada {
			// A base trocou no meio da busca: nada foi extraído ainda, então os
			// candidatos escolhidos são descartados e a busca recomeça na versão nova.
			if est.Refeitas >= gerarListaMaxRefazer {
				return Fatia{}, errors.New("a base de empresas foi atualizada durante a busca; gere a lista de novo")
			}
			res := ResultadoGerarLista{Pedidos: est.Res.Pedidos, LimitadoPelaCota: est.Res.LimitadoPelaCota}
			*est = estadoGerarLista{Fase: faseGerarCandidatos, Quantidade: est.Quantidade, Refeitas: est.Refeitas + 1, Res: res}
			if err := e.SalvarEstado(est); err != nil {
				return Fatia{}, Transitorio(err)
			}
			return Fatia{Mensagem: "a base foi atualizada; refazendo a busca"}, nil
		}
		return Fatia{}, err
	}
	escolhidos, err := gerarListaFiltrar(e.DB, lista, est, pagina.CNPJs)
	if err != nil {
		return Fatia{}, Transitorio(err)
	}
	est.Escolhidos = escolhidos
	est.Res.CandidatosLidos += len(pagina.CNPJs)
	est.Cursor = pagina.Cursor
	if pagina.Versao != "" {
		est.Versao = pagina.Versao
	}
	acabou := pagina.Fim || len(pagina.CNPJs) == 0 || pagina.Cursor == "" ||
		len(est.Escolhidos) >= est.Quantidade ||
		est.Res.CandidatosLidos >= gerarListaTetoLeitura*est.Quantidade
	if acabou {
		return gerarListaFecharCandidatos(e, lista, est)
	}
	if err := e.SalvarEstado(est); err != nil {
		return Fatia{}, Transitorio(err)
	}
	return Fatia{Mensagem: fmt.Sprintf("buscando empresas na base: %d de %d escolhidas", len(est.Escolhidos), est.Quantidade)}, nil
}

// gerarListaFecharCandidatos fatia os escolhidos em lotes e GRAVA o estado
// antes de qualquer extração: é o que garante que a retomada reenvia os mesmos
// lotes e a cota não é descontada duas vezes.
func gerarListaFecharCandidatos(e *ExecucaoJob, lista *models.ProspLista, est *estadoGerarLista) (Fatia, error) {
	est.Lotes = nil
	for ini := 0; ini < len(est.Escolhidos); ini += GerarListaLote {
		est.Lotes = append(est.Lotes, est.Escolhidos[ini:min(ini+GerarListaLote, len(est.Escolhidos))])
	}
	total := len(est.Escolhidos)
	est.Escolhidos, est.Cursor = nil, ""
	est.Fase, est.Lote = faseGerarExtracao, 0
	if err := e.DefinirTotal(total); err != nil {
		return Fatia{}, Transitorio(err)
	}
	if err := e.SalvarEstado(est); err != nil {
		return Fatia{}, Transitorio(err)
	}
	if total == 0 {
		return gerarListaConcluir(e, lista, est, "")
	}
	return Fatia{Mensagem: fmt.Sprintf("extraindo %d empresas", total)}, nil
}

// gerarListaExtracao extrai um lote por fatia e grava os itens.
func gerarListaExtracao(ctx context.Context, e *ExecucaoJob, lista *models.ProspLista, est *estadoGerarLista) (Fatia, error) {
	if est.Lote >= len(est.Lotes) {
		return gerarListaConcluir(e, lista, est, "")
	}
	lote := est.Lotes[est.Lote]
	resp, err := PlataformaExtracao(ctx, lista.PedidoID, lote, lista.IncluirPF)
	if err != nil {
		p, ok := ComoErroPlataforma(err)
		if !ok || p.Codigo != CodigoCotaEsgotada {
			return Fatia{}, err
		}
		// Sem saldo para o lote inteiro: nada foi entregue. Se ainda resta
		// saldo, o lote encolhe para o que resta (gravado antes de pedir de
		// novo) e passa a ser o último; senão o job termina aqui.
		if p.Cota != nil && p.Cota.Restante > 0 && p.Cota.Restante < len(lote) {
			est.Lotes[est.Lote] = lote[:p.Cota.Restante]
			est.Lotes = est.Lotes[:est.Lote+1]
			est.Res.LimitadoPelaCota = true
			if err := e.SalvarEstado(est); err != nil {
				return Fatia{}, Transitorio(err)
			}
			return Fatia{Mensagem: "cota quase no fim: extraindo o que resta"}, nil
		}
		est.Res.LimitadoPelaCota = true
		entregues, err := gerarListaEntregues(e.DB, lista.ID)
		if err != nil {
			return Fatia{}, Transitorio(err)
		}
		return gerarListaConcluir(e, lista, est, fmt.Sprintf("cota esgotada: %d de %d entregues", entregues, est.Res.Pedidos))
	}

	itens := make([]models.ProspItem, 0, len(resp.Itens))
	puladosPF := 0
	for i := range resp.Itens {
		emp := resp.Itens[i]
		if emp.PessoaFisica && !lista.IncluirPF {
			// Pessoa física só entra em lista criada com incluir_pf.
			puladosPF++
			continue
		}
		itens = append(itens, ItemDaEmpresa(&emp))
	}
	inseridos, err := models.InsertProspItens(e.DB, lista.ID, itens)
	if err != nil {
		return Fatia{}, Transitorio(err)
	}
	est.Res.PuladosPF += puladosPF
	est.Res.NaoEncontrados += len(resp.NaoEncontrados)
	est.Lote++
	if err := e.SalvarEstado(est); err != nil {
		return Fatia{}, Transitorio(err)
	}
	if err := e.Progresso(inseridos, 0, 0); err != nil {
		return Fatia{}, Transitorio(err)
	}
	if est.Lote >= len(est.Lotes) {
		return gerarListaConcluir(e, lista, est, "")
	}
	return Fatia{Mensagem: fmt.Sprintf("extraindo empresas: lote %d de %d", est.Lote+1, len(est.Lotes))}, nil
}

// ItemDaEmpresa monta o item da lista a partir da ficha da base pública:
// retrato, nota com os motivos e marcas.
func ItemDaEmpresa(emp *models.ProspEmpresa) models.ProspItem {
	nota := CalcularNotaItem(emp, nil)
	return models.ProspItem{
		CNPJ:                 models.NormalizarCNPJ(emp.CNPJ),
		RazaoSocial:          emp.RazaoSocial,
		NomeFantasia:         emp.NomeFantasia,
		UF:                   emp.Endereco.UF,
		Municipio:            emp.Endereco.Municipio,
		CnaeCodigo:           emp.CNAE.Codigo,
		CnaeDescricao:        emp.CNAE.Descricao,
		Porte:                emp.Porte,
		PessoaFisica:         emp.PessoaFisica,
		Nota:                 nota.Nota,
		NotaMotivos:          nota.Motivos,
		TemCelular:           nota.TemCelular,
		TemEmail:             nota.TemEmail,
		TemDecisor:           nota.TemDecisor,
		TemWhatsApp:          nota.TemWhatsApp,
		TemDivida:            nota.TemDivida,
		ContatoCompartilhado: nota.ContatoCompartilhado,
		TelefonePrincipal:    nota.TelefonePrincipal,
		Dados:                emp,
	}
}

func gerarListaEntregues(db models.Executor, listaID int64) (int, error) {
	c, err := models.ProspContadoresDaLista(db, listaID)
	return c.Total, err
}

// gerarListaConcluir fecha a lista e o job. motivoParcial, se não vazio, é a
// mensagem do job (ex.: cota esgotada).
func gerarListaConcluir(e *ExecucaoJob, lista *models.ProspLista, est *estadoGerarLista, motivoParcial string) (Fatia, error) {
	cont, err := models.ProspContadoresDaLista(e.DB, lista.ID)
	if err != nil {
		return Fatia{}, Transitorio(err)
	}
	res := est.Res
	res.Entregues = cont.Total
	msg := mensagemListaGerada(res, motivoParcial)
	status := models.ListaPronta
	if cont.Total == 0 && motivoParcial != "" {
		status = models.ListaFalhou // nada entregue por falta de cota
	}
	if err := models.SetProspListaStatus(e.DB, lista.ID, status, msg, est.Versao); err != nil {
		return Fatia{}, Transitorio(err)
	}
	if cont.Total > 0 {
		auditarListaGerada(e, lista, res, cont.PessoaFisica)
	}
	if motivoParcial != "" {
		return Fatia{Concluido: true, Mensagem: motivoParcial, Resultado: res}, nil
	}
	return Fatia{Concluido: true, Mensagem: msg, Resultado: res}, nil
}

// mensagemListaGerada explica, para a tela, por que a lista saiu como saiu.
func mensagemListaGerada(res ResultadoGerarLista, motivoParcial string) string {
	var msg string
	switch {
	case motivoParcial != "":
		msg = motivoParcial
	case res.Entregues == 0:
		msg = "nenhuma empresa nova encontrada para este filtro"
	case res.Entregues < res.Pedidos:
		msg = fmt.Sprintf("a lista saiu menor que o pedido: %d de %d empresas", res.Entregues, res.Pedidos)
		if res.LimitadoPelaCota {
			msg += " (limitada pela cota do mês)"
		}
	default:
		msg = fmt.Sprintf("%d empresas entregues", res.Entregues)
	}
	var extras []string
	add := func(n int, um, varios string) {
		if n == 1 {
			extras = append(extras, "1 "+um)
		} else if n > 1 {
			extras = append(extras, fmt.Sprintf("%d %s", n, varios))
		}
	}
	add(res.PuladosNoCRM, "já estava no CRM", "já estavam no CRM")
	add(res.PuladosEmListas, "já estava em outra lista", "já estavam em outras listas")
	add(res.PuladosBloqueio, "bloqueada", "bloqueadas")
	add(res.PuladosPF, "de pessoa física (MEI ou empresário individual) pulada", "de pessoa física (MEI ou empresário individual) puladas")
	add(res.NaoEncontrados, "não encontrada na base", "não encontradas na base")
	for i, x := range extras {
		if i == 0 {
			msg += "; "
		} else {
			msg += ", "
		}
		msg += x
	}
	return msg
}

// auditarListaGerada grava "lista gerada" na trilha, com os filtros, o total e
// quantos itens são de pessoa física. Falha de auditoria não derruba o job.
func auditarListaGerada(e *ExecucaoJob, lista *models.ProspLista, res ResultadoGerarLista, pessoaFisica int) {
	detalhes, _ := json.Marshal(map[string]any{
		"filtros":       lista.Filtros,
		"origem":        lista.Origem,
		"ordem":         lista.Ordem,
		"pedidos":       res.Pedidos,
		"total":         res.Entregues,
		"pessoa_fisica": pessoaFisica,
		"incluir_pf":    lista.IncluirPF,
		"versao_base":   lista.VersaoBase,
	})
	resumo := fmt.Sprintf("lista gerada: %q com %d empresas", lista.Nome, res.Entregues)
	if lista.IncluirPF {
		resumo += fmt.Sprintf(" (inclui pessoa física: %d)", pessoaFisica)
	}
	entrada := models.AuditEntry{
		UserID: e.UserID(), Action: models.AuditCreate, Entity: EntidadeListaProspeccao,
		EntityID: &lista.ID, Summary: resumo, Details: detalhes,
	}
	if entrada.UserID != nil {
		_ = e.DB.QueryRow(`SELECT name FROM users WHERE id = $1`, *entrada.UserID).Scan(&entrada.UserName)
	}
	if err := models.RecordAudit(e.DB, &entrada); err != nil {
		log.Printf("prospecção: falha ao registrar auditoria da lista %d: %v", lista.ID, err)
	}
}

// Encerrar arruma a lista quando o job termina em "falhou" ou "cancelado":
// com itens já gravados a lista fica "pronta" e a mensagem diz que veio
// parcial (a cota desses itens já foi descontada); sem nenhum item, "falhou".
func (processadorGerarLista) Encerrar(ctx context.Context, e *ExecucaoJob, status string) (string, any, error) {
	if e.Job.ListaID == nil {
		return "", nil, nil
	}
	lista, err := models.ProspListaByID(e.DB, *e.Job.ListaID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil, nil
		}
		return "", nil, err
	}
	var est estadoGerarLista
	_ = e.Estado(&est)
	res := est.Res
	if res.Pedidos == 0 {
		res.Pedidos = lista.QuantidadePedida
	}
	res.Entregues = lista.Contadores.Total

	causa := "geração cancelada"
	if status == models.JobFalhou {
		causa = "a geração foi interrompida"
		if j, err := models.ProspJobByID(e.DB, e.Job.ID); err == nil && j.Mensagem != "" {
			causa += ": " + j.Mensagem
		}
	}
	if res.Entregues == 0 {
		msg := causa + " antes de entregar empresas"
		if status == models.JobFalhou {
			msg = causa
		}
		return "", res, models.SetProspListaStatus(e.DB, lista.ID, models.ListaFalhou, msg, "")
	}
	msg := fmt.Sprintf("lista parcial: %d de %d empresas (%s)", res.Entregues, res.Pedidos, causa)
	if err := models.SetProspListaStatus(e.DB, lista.ID, models.ListaPronta, msg, est.Versao); err != nil {
		return "", res, err
	}
	auditarListaGerada(e, lista, res, lista.Contadores.PessoaFisica)
	if status == models.JobCancelado {
		return fmt.Sprintf("cancelado pelo usuário: %d de %d empresas entregues", res.Entregues, res.Pedidos), res, nil
	}
	return "", res, nil
}

// EntidadeListaProspeccao é o nome da entidade na trilha de auditoria.
const EntidadeListaProspeccao = "lista_prospeccao"
