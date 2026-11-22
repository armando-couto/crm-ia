package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services/enriquecimento"
)

// -----------------------------------------------------------------------------
// Enriquecimento em lote das listas de prospecção pelo Who Lead
// (docs/PROSPECCAO.md seção 7, com as correções do adendo).
//
// O handler HTTP só confere a estimativa e insere o job; o trabalho é do
// processador "enriquecer", em fatias curtas:
//
//  1. pega a próxima página da seleção (itens novo, sem_resultado ou erro; e
//     enriquecido com reprocessar), em ordem de id, até lote_tamanho;
//  2. POST /enrichment e grava o consultation_id no estado do job ANTES de
//     qualquer outra coisa (a retomada nunca paga duas vezes);
//  3. consulta o andamento com espera crescente (5 s até 30 s);
//  4. lê os resultados em páginas de 500 e, numa transação por página, grava
//     os itens (enriquecido | sem_resultado | erro, perfil, marcas, nota,
//     custo), soma o progresso e o custo do job e avança o cursor;
//  5. no fim, auditoria "lista enriquecida" com provedor, quantidade e custo.
//
// 402 (saldo ou franquia esgotados) é estado de negócio: o job para como
// concluído, com a mensagem, e os itens que faltam ficam como estavam.
// Cancelar (ou falhar) com um lote já pago em aberto ainda lê e grava esse
// lote (Encerrar). O custo fica em prospeccao_jobs.custo_centavos, que o uso
// mensal soma (models.EnrichmentUsage). Demonstração custa 0.
// -----------------------------------------------------------------------------

func init() { RegistrarProcessadorJob(models.JobEnriquecer, processadorEnriquecer{}) }

// EnriquecerEsperas é a espera entre consultas ao andamento do lote (cresce
// até a última e fica nela). Variável para os testes zerarem.
var EnriquecerEsperas = []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second,
	20 * time.Second, 25 * time.Second, 30 * time.Second}

// enriquecerDemoPorFatia limita quantos itens a demonstração grava por fatia.
const enriquecerDemoPorFatia = 200

// enriqEntidadeAuditoria é a entidade da trilha de auditoria das listas.
const enriqEntidadeAuditoria = "lista_prospeccao"

// ParametrosEnriquecer é a entrada do job (gravada pelo handler).
type ParametrosEnriquecer struct {
	Selecao               models.ProspSelecao `json:"selecao"`
	Reprocessar           bool                `json:"reprocessar"`
	Demo                  bool                `json:"demo"`
	LoteTamanho           int                 `json:"lote_tamanho"`
	CustoUnitarioCentavos int                 `json:"custo_unitario_centavos"`
	CustoEstimadoCentavos int                 `json:"custo_estimado_centavos"`
}

// ResultadoEnriquecer é o resumo do job (também o cursor dos totais).
type ResultadoEnriquecer struct {
	Provedor      string `json:"provedor"`
	Demo          bool   `json:"demo"`
	Processados   int    `json:"processados"`
	Enriquecidos  int    `json:"enriquecidos"`
	SemResultado  int    `json:"sem_resultado"`
	Erros         int    `json:"erros"`
	CustoCentavos int    `json:"custo_centavos"`
	Lotes         int    `json:"lotes"`
	// Parado explica por que o job parou antes do fim: "saldo" (402).
	Parado string `json:"parado,omitempty"`
}

// enriqLote é o lote pago que ainda não foi lido por inteiro.
type enriqLote struct {
	ConsultationID int64 `json:"consultation_id"`
	// Itens liga o CNPJ normalizado ao id do item; quem já foi gravado sai.
	Itens          map[string]int64 `json:"itens"`
	Ordem          []string         `json:"ordem"` // CNPJs na ordem enviada
	Quantidade     int              `json:"quantidade"`
	CustoCentavos  int              `json:"custo_centavos"`
	CustoInformado bool             `json:"custo_informado"`
	Pronto         bool             `json:"pronto"`
	Offset         int              `json:"offset"`
	Consultas      int              `json:"consultas"` // quantas vezes o andamento foi consultado
}

// enriqEstado é o cursor de retomada.
type enriqEstado struct {
	AposID int64               `json:"apos_id"`
	Lote   *enriqLote          `json:"lote,omitempty"`
	Res    ResultadoEnriquecer `json:"res"`
}

type processadorEnriquecer struct{}

// enriqEstados são os estados que a seleção alcança.
func enriqEstados(reprocessar bool) []string {
	estados := append([]string{}, models.ItemEstadosAEnriquecer...)
	if reprocessar {
		estados = append(estados, models.ItemEnriquecido)
	}
	return estados
}

func (processadorEnriquecer) Fatia(ctx context.Context, e *ExecucaoJob) (Fatia, error) {
	if e.Job.ListaID == nil {
		return Fatia{}, errors.New("job de enriquecimento sem lista")
	}
	var par ParametrosEnriquecer
	if err := e.Parametros(&par); err != nil {
		return Fatia{}, fmt.Errorf("parâmetros do enriquecimento ilegíveis: %w", err)
	}
	var est enriqEstado
	if err := e.Estado(&est); err != nil {
		return Fatia{}, fmt.Errorf("estado do enriquecimento ilegível: %w", err)
	}
	est.Res.Provedor, est.Res.Demo = models.ProvedorWhoLead, par.Demo
	listaID := *e.Job.ListaID

	if est.Lote != nil {
		w, err := enriqClienteWhoLead(e.DB)
		if err != nil {
			return Fatia{}, err
		}
		return enriqAvancarLote(ctx, e, w, &est)
	}

	tamanho := par.LoteTamanho
	if tamanho <= 0 {
		tamanho = models.WhoLeadLotePadrao
	}
	if par.Demo {
		tamanho = min(tamanho, enriquecerDemoPorFatia)
	}
	ids, err := models.ResolveProspSelecao(e.DB, listaID, par.Selecao, enriqEstados(par.Reprocessar), est.AposID, tamanho)
	if err != nil {
		return Fatia{}, err
	}
	if len(ids) == 0 {
		return enriqConcluir(e, listaID, &est, "")
	}
	itens, err := models.ProspItensCompletos(e.DB, ids)
	if err != nil {
		return Fatia{}, err
	}
	if par.Demo {
		if err := enriqDemo(ctx, e, itens, &est, ids[len(ids)-1]); err != nil {
			return Fatia{}, err
		}
		return Fatia{Mensagem: enriqMensagem(e, est.Res)}, nil
	}

	w, err := enriqClienteWhoLead(e.DB)
	if err != nil {
		return Fatia{}, err
	}
	lote := &enriqLote{Itens: map[string]int64{}, Ordem: []string{}}
	for _, it := range itens {
		c := models.NormalizarCNPJ(it.CNPJ)
		if _, repetido := lote.Itens[c]; repetido || c == "" {
			continue
		}
		lote.Itens[c] = it.ID
		lote.Ordem = append(lote.Ordem, c)
	}
	criado, err := w.CriarLote(ctx, lote.Ordem)
	if err != nil {
		if enriquecimento.TipoDaFalha(err) == enriquecimento.FalhaSaldo {
			est.Res.Parado = "saldo"
			return enriqConcluir(e, listaID, &est, enriquecimento.MensagemDaFalha(err))
		}
		return Fatia{}, enriqErroDoProvedor(err)
	}
	// O lote já foi cobrado: o consultation_id vai para o estado antes de
	// qualquer outra coisa. Uma falha aqui é transitória e a retomada não tem
	// como saber do lote: fica no log para conferência.
	lote.ConsultationID = criado.ConsultationID
	lote.Quantidade = len(lote.Ordem)
	if criado.Itens > 0 {
		lote.Quantidade = criado.Itens
	}
	lote.CustoCentavos, lote.CustoInformado = criado.CustoCentavos, criado.CustoInformado
	if !lote.CustoInformado {
		lote.CustoCentavos = par.CustoUnitarioCentavos * lote.Quantidade
	}
	est.Lote = lote
	est.AposID = ids[len(ids)-1]
	est.Res.Lotes++
	if err := e.SalvarEstado(est); err != nil {
		log.Printf("prospecção: job %d: lote %d do Who Lead criado e NÃO gravado no estado: %v", e.Job.ID, lote.ConsultationID, err)
		return Fatia{}, Transitorio(err)
	}
	return Fatia{Mensagem: fmt.Sprintf("lote %d enviado ao Who Lead (%d empresas); aguardando o processamento", est.Res.Lotes, len(lote.Ordem))}, nil
}

// Encerrar, em "cancelado" ou "falhou", ainda lê e grava o lote já pago que
// estava em aberto, e registra a auditoria do que foi enriquecido.
func (processadorEnriquecer) Encerrar(ctx context.Context, e *ExecucaoJob, status string) (string, any, error) {
	var par ParametrosEnriquecer
	_ = e.Parametros(&par)
	var est enriqEstado
	if err := e.Estado(&est); err != nil {
		return "", nil, err
	}
	est.Res.Provedor, est.Res.Demo = models.ProvedorWhoLead, par.Demo
	var errLote error
	if est.Lote != nil {
		errLote = enriqLerLoteAteOFim(ctx, e, &est)
	}
	if e.Job.ListaID != nil && est.Res.Processados > 0 {
		enriqAuditar(e, *e.Job.ListaID, est.Res)
	}
	msg := ""
	if status == models.JobCancelado {
		msg = fmt.Sprintf("cancelado pelo usuário: %d enriquecidos, %d sem resultado, %d com erro", est.Res.Enriquecidos, est.Res.SemResultado, est.Res.Erros)
		if errLote != nil {
			msg += "; o lote já pago não pôde ser lido: " + enriquecimento.MensagemDaFalha(errLote)
		}
	}
	return msg, est.Res, errLote
}

// enriqLerLoteAteOFim espera o lote ficar pronto (dentro do prazo do contexto) e
// grava todas as páginas.
func enriqLerLoteAteOFim(ctx context.Context, e *ExecucaoJob, est *enriqEstado) error {
	w, err := enriqClienteWhoLead(e.DB)
	if err != nil {
		return err
	}
	for tentativa := 0; est.Lote != nil && tentativa < 200; tentativa++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		fatia, err := enriqAvancarLote(ctx, e, w, est)
		if err != nil {
			return err
		}
		if fatia.Espera > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(fatia.Espera):
			}
		}
	}
	return nil
}

// enriqClienteWhoLead monta o cliente real com as configurações atuais.
func enriqClienteWhoLead(db *sql.DB) (*enriquecimento.WhoLead, error) {
	return enriquecimento.NovoRegistro(models.LoadEnrichmentSettings(db)).WhoLead()
}

// enriqErroDoProvedor marca como transitório o que vale tentar de novo (rede,
// 409/429, 5xx do Who Lead); credencial, configuração e pedido recusado
// encerram o job.
func enriqErroDoProvedor(err error) error {
	switch enriquecimento.TipoDaFalha(err) {
	case enriquecimento.FalhaExterna, enriquecimento.FalhaLimite, enriquecimento.FalhaOcupado:
		return Transitorio(err)
	}
	return err
}

// enriqAvancarLote dá um passo no lote em curso: consulta o andamento ou lê e
// grava uma página de resultados.
func enriqAvancarLote(ctx context.Context, e *ExecucaoJob, w *enriquecimento.WhoLead, est *enriqEstado) (Fatia, error) {
	lote := est.Lote
	if !lote.Pronto {
		and, err := w.Andamento(ctx, lote.ConsultationID)
		if err != nil {
			return Fatia{}, enriqErroDoProvedor(err)
		}
		switch {
		case and.Status == enriquecimento.LoteFalhou:
			// O provedor não processou: os itens ficam em erro (o débito do
			// lote que falhou é estornado pelo provedor).
			if err := enriqFecharLoteSemResposta(ctx, e, est, "o Who Lead não conseguiu processar o lote; tente de novo"); err != nil {
				return Fatia{}, err
			}
			return Fatia{Mensagem: enriqMensagem(e, est.Res)}, nil
		case !and.Pronto:
			lote.Consultas++
			if err := e.SalvarEstado(est); err != nil {
				return Fatia{}, err
			}
			espera := time.Duration(0)
			if len(EnriquecerEsperas) > 0 {
				espera = EnriquecerEsperas[min(lote.Consultas, len(EnriquecerEsperas))-1]
			}
			msg := "aguardando o Who Lead processar o lote"
			if and.Total > 0 {
				msg = fmt.Sprintf("aguardando o Who Lead processar o lote (%d de %d)", and.Processados, and.Total)
			}
			return Fatia{Espera: espera, Mensagem: msg}, nil
		}
		lote.Pronto = true
		if err := e.SalvarEstado(est); err != nil {
			return Fatia{}, err
		}
	}

	pag, err := w.Resultados(ctx, lote.ConsultationID, lote.Offset, enriquecimento.WhoLeadResultadosLimite)
	if err != nil {
		return Fatia{}, enriqErroDoProvedor(err)
	}
	// Fim do lote: página vazia, ou o total informado alcançado (sem total,
	// página menor que o limite).
	fim := len(pag.Itens) == 0
	if pag.Total > 0 {
		fim = fim || lote.Offset+len(pag.Itens) >= pag.Total
	} else {
		fim = fim || len(pag.Itens) < enriquecimento.WhoLeadResultadosLimite
	}
	if err := enriqGravarPagina(ctx, e, est, pag.Itens, fim); err != nil {
		return Fatia{}, err
	}
	return Fatia{Mensagem: enriqMensagem(e, est.Res)}, nil
}

// enriqCustoDoItem distribui o custo total do lote pelos itens sem perder centavo:
// o item na posição k (0..n-1) custa floor(T(k+1)/n) − floor(Tk/n).
func enriqCustoDoItem(total, n, k int) int {
	if n <= 0 || total <= 0 || k < 0 || k >= n {
		return 0
	}
	return total*(k+1)/n - total*k/n
}

// enriqGravacao é o que uma página grava num item.
type enriqGravacao struct {
	id  int64
	upd models.ProspItemEnriquecimento
}

// enriqGravarPagina grava uma página de resultados numa transação:
// itens, progresso e custo do job e o cursor andam juntos (repetir a página
// depois de uma queda não soma custo duas vezes). Com fim = true, itens do
// lote que não vieram nos resultados ficam em erro e o lote é fechado.
func enriqGravarPagina(ctx context.Context, e *ExecucaoJob, est *enriqEstado, itens []enriquecimento.ResultadoWhoLead, fim bool) error {
	lote := est.Lote
	posicao := map[string]int{}
	for i, c := range lote.Ordem {
		posicao[c] = i
	}
	ids := []int64{}
	porCNPJ := map[string]enriquecimento.ResultadoWhoLead{}
	for _, r := range itens {
		if id, ok := lote.Itens[r.CNPJ]; ok {
			if _, visto := porCNPJ[r.CNPJ]; !visto {
				ids = append(ids, id)
			}
			porCNPJ[r.CNPJ] = r
		}
	}
	atuais, err := models.ProspItensCompletos(e.DB, ids)
	if err != nil {
		return err
	}
	porID := map[int64]models.ProspItem{}
	for _, it := range atuais {
		porID[it.ID] = it
	}
	agora := time.Now()
	gravacoes := []enriqGravacao{}
	res := est.Res
	custoPagina := 0
	for c, r := range porCNPJ {
		id := lote.Itens[c]
		it, ok := porID[id]
		if !ok {
			delete(lote.Itens, c) // item apagado no meio do caminho
			continue
		}
		custo := 0
		if !r.Estornado {
			custo = enriqCustoDoItem(lote.CustoCentavos, lote.Quantidade, posicao[c])
		}
		upd := models.ProspItemEnriquecimento{Provedor: models.ProvedorWhoLead, CustoCentavos: custo, Bruto: r.Bruto}
		switch r.Resultado {
		case enriquecimento.ResultadoEncontrado:
			pc, _, _ := enriquecimento.PerfilContatoDeResultado(r, agora)
			upd.Estado, upd.Perfil = models.ItemEnriquecido, pc
			enriqPreencherNota(&upd, it.Dados, pc)
			res.Enriquecidos++
		case enriquecimento.ResultadoNaoEncontrado:
			upd.Estado = models.ItemSemResultado
			enriqPreencherNota(&upd, it.Dados, it.Enriquecimento)
			res.SemResultado++
		default:
			upd.Estado = models.ItemErro
			upd.Erro = "o Who Lead não conseguiu consultar esta empresa agora; tente de novo"
			if r.Resultado == enriquecimento.ResultadoLimitado {
				upd.Erro = "limite de consultas da fonte do Who Lead atingido; tente de novo mais tarde"
			}
			res.Erros++
		}
		res.Processados++
		custoPagina += custo
		gravacoes = append(gravacoes, enriqGravacao{id: id, upd: upd})
		delete(lote.Itens, c)
	}
	errosFim := 0
	if fim {
		for _, id := range lote.Itens {
			gravacoes = append(gravacoes, enriqGravacao{id: id, upd: models.ProspItemEnriquecimento{
				Estado: models.ItemErro, Erro: "o Who Lead não devolveu resultado para esta empresa; tente de novo",
			}})
			errosFim++
		}
		res.Erros += errosFim
		res.Processados += errosFim
	}
	res.CustoCentavos += custoPagina
	novo := *est
	novo.Res = res
	if fim {
		novo.Lote = nil
	} else {
		l := *lote
		l.Offset += len(itens)
		novo.Lote = &l
	}
	erros := res.Erros - est.Res.Erros
	feitos := res.Processados - est.Res.Processados
	if err := enriqGravarTransacao(ctx, e, gravacoes, novo, feitos, erros, custoPagina); err != nil {
		return err
	}
	*est = novo
	return nil
}

// enriqFecharLoteSemResposta fecha o lote com todos os itens restantes em erro.
func enriqFecharLoteSemResposta(ctx context.Context, e *ExecucaoJob, est *enriqEstado, motivo string) error {
	gravacoes := []enriqGravacao{}
	for _, id := range est.Lote.Itens {
		gravacoes = append(gravacoes, enriqGravacao{id: id, upd: models.ProspItemEnriquecimento{Estado: models.ItemErro, Erro: motivo}})
	}
	novo := *est
	novo.Lote = nil
	novo.Res.Erros += len(gravacoes)
	novo.Res.Processados += len(gravacoes)
	if err := enriqGravarTransacao(ctx, e, gravacoes, novo, len(gravacoes), len(gravacoes), 0); err != nil {
		return err
	}
	*est = novo
	return nil
}

// enriqGravarTransacao grava itens, progresso e estado do job de uma vez.
func enriqGravarTransacao(ctx context.Context, e *ExecucaoJob, gravacoes []enriqGravacao, est enriqEstado, feitos, erros, custo int) error {
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // depois do Commit não faz nada
	for _, g := range gravacoes {
		if err := models.UpdateProspItemEnriquecimento(tx, g.id, g.upd); err != nil {
			return fmt.Errorf("gravar item %d: %w", g.id, err)
		}
	}
	if feitos != 0 || erros != 0 || custo != 0 {
		if err := models.SomarProspJobProgresso(tx, e.Job.ID, feitos, erros, custo); err != nil {
			return err
		}
	}
	b, err := json.Marshal(est)
	if err != nil {
		return err
	}
	if err := models.SetProspJobEstado(tx, e.Job.ID, json.RawMessage(b)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	e.Job.Estado = b
	e.Job.Feitos += feitos
	e.Job.Erros += erros
	e.Job.CustoCentavos += custo
	return nil
}

// enriqPreencherNota recalcula nota e marcas do item com o perfil final.
func enriqPreencherNota(upd *models.ProspItemEnriquecimento, dados *models.ProspEmpresa, perfil *models.PerfilContato) {
	n := CalcularNotaItem(dados, perfil)
	upd.Nota, upd.NotaMotivos = n.Nota, n.Motivos
	upd.TemCelular, upd.TemEmail, upd.TemDecisor, upd.TemWhatsApp = n.TemCelular, n.TemEmail, n.TemDecisor, n.TemWhatsApp
	upd.ContatoCompartilhado, upd.TelefonePrincipal = n.ContatoCompartilhado, n.TelefonePrincipal
}

// enriquecerDemo grava contatos fictícios (custo zero) numa transação.
func enriqDemo(ctx context.Context, e *ExecucaoJob, itens []models.ProspItem, est *enriqEstado, ultimoID int64) error {
	gravacoes := make([]enriqGravacao, 0, len(itens))
	novo := *est
	for _, it := range itens {
		pc, err := enriquecimento.PerfilContatoDemo(it.CNPJ)
		upd := models.ProspItemEnriquecimento{Provedor: models.ProvedorWhoLead}
		if err != nil {
			upd.Estado, upd.Erro = models.ItemErro, enriquecimento.MensagemDaFalha(err)
			novo.Res.Erros++
		} else {
			upd.Estado, upd.Perfil = models.ItemEnriquecido, pc
			enriqPreencherNota(&upd, it.Dados, pc)
			novo.Res.Enriquecidos++
		}
		novo.Res.Processados++
		gravacoes = append(gravacoes, enriqGravacao{id: it.ID, upd: upd})
	}
	novo.AposID = ultimoID
	if err := enriqGravarTransacao(ctx, e, gravacoes, novo, novo.Res.Processados-est.Res.Processados, novo.Res.Erros-est.Res.Erros, 0); err != nil {
		return err
	}
	*est = novo
	return nil
}

func enriqMensagem(e *ExecucaoJob, r ResultadoEnriquecer) string {
	if e.Job.Total > 0 {
		return fmt.Sprintf("enriquecendo: %d de %d empresas", r.Processados, e.Job.Total)
	}
	return fmt.Sprintf("enriquecendo: %d empresas", r.Processados)
}

// enriqConcluir encerra o job com o resumo e a auditoria.
func enriqConcluir(e *ExecucaoJob, listaID int64, est *enriqEstado, motivoParada string) (Fatia, error) {
	r := est.Res
	if r.Processados > 0 {
		enriqAuditar(e, listaID, r)
	}
	msg := fmt.Sprintf("%d enriquecidos, %d sem resultado, %d com erro", r.Enriquecidos, r.SemResultado, r.Erros)
	if r.Demo {
		msg += " (dados de demonstração)"
	} else if r.CustoCentavos > 0 {
		msg += fmt.Sprintf("; custo R$ %s", enriqReais(r.CustoCentavos))
	}
	if motivoParada != "" {
		msg = motivoParada + " Parcial: " + msg
	}
	return Fatia{Concluido: true, Mensagem: msg, Resultado: r}, nil
}

// enriqReais formata 1234 como "12,34".
func enriqReais(c int) string {
	sinal := ""
	if c < 0 {
		sinal, c = "-", -c
	}
	return fmt.Sprintf("%s%d,%02d", sinal, c/100, c%100)
}

// enriqAuditar grava "lista enriquecida" com provedor, quantidade
// e custo. Falha de auditoria não derruba o job.
func enriqAuditar(e *ExecucaoJob, listaID int64, r ResultadoEnriquecer) {
	nome := ""
	if l, err := models.ProspListaByID(e.DB, listaID); err == nil {
		nome = l.Nome
	}
	detalhes, _ := json.Marshal(map[string]any{
		"job_id": e.Job.ID, "provedor": r.Provedor, "demo": r.Demo, "processados": r.Processados,
		"enriquecidos": r.Enriquecidos, "sem_resultado": r.SemResultado, "erros": r.Erros,
		"custo_centavos": r.CustoCentavos, "lotes": r.Lotes, "parado": r.Parado,
	})
	resumo := fmt.Sprintf("lista enriquecida: %q, %d empresas pelo %s (custo R$ %s)", nome, r.Processados,
		models.NomeProvedor(r.Provedor), enriqReais(r.CustoCentavos))
	if r.Demo {
		resumo += " — demonstração"
	}
	entrada := models.AuditEntry{UserID: e.UserID(), Action: models.AuditUpdate, Entity: enriqEntidadeAuditoria,
		EntityID: &listaID, Summary: enriqCortarResumo(resumo), Details: detalhes}
	if entrada.UserID != nil {
		_ = e.DB.QueryRow(`SELECT name FROM users WHERE id = $1`, *entrada.UserID).Scan(&entrada.UserName)
	}
	if err := models.RecordAudit(e.DB, &entrada); err != nil {
		log.Printf("prospecção: falha ao registrar auditoria do enriquecimento da lista %d: %v", listaID, err)
	}
}

// enriqCortarResumo respeita audit_log.summary VARCHAR(300).
func enriqCortarResumo(s string) string {
	return enriquecimento.TruncarTexto(s, 300)
}

// -----------------------------------------------------------------------------
// Estimativa e criação do job (usadas pelo controller)
// -----------------------------------------------------------------------------

// ProvedorEstimativa descreve o provedor na estimativa.
type ProvedorEstimativa struct {
	Codigo string `json:"codigo"`
	Nome   string `json:"nome"`
	Pronto bool   `json:"pronto"`
	Demo   bool   `json:"demo"`
	Motivo string `json:"motivo"`
}

// UsoMesEstimativa é o consumo do mês com o limite.
type UsoMesEstimativa struct {
	CustoCentavos  int `json:"custo_centavos"`
	LimiteCentavos int `json:"limite_centavos"`
}

// SaldoEstimativa é o plano e a franquia no Who Lead (nil sem conta real).
type SaldoEstimativa struct {
	Plano            string `json:"plano"`
	FranquiaTotal    *int   `json:"franquia_total"`
	FranquiaRestante *int   `json:"franquia_restante"`
}

// EnriquecerEstimativa é a resposta da estimativa (interface do contrato).
type EnriquecerEstimativa struct {
	Selecionados          int                `json:"selecionados"`
	AEnriquecer           int                `json:"a_enriquecer"`
	JaEnriquecidos        int                `json:"ja_enriquecidos"`
	Provedor              ProvedorEstimativa `json:"provedor"`
	CustoUnitarioCentavos int                `json:"custo_unitario_centavos"`
	CustoEstimadoCentavos int                `json:"custo_estimado_centavos"`
	UsoMes                UsoMesEstimativa   `json:"uso_mes"`
	Saldo                 *SaldoEstimativa   `json:"saldo"`

	cfg models.EnrichmentSettings
}

// EstourariaLimite diz se a estimativa passa do limite mensal configurado.
func (est *EnriquecerEstimativa) EstourariaLimite() bool {
	return est.UsoMes.LimiteCentavos > 0 && est.CustoEstimadoCentavos > 0 &&
		est.UsoMes.CustoCentavos+est.CustoEstimadoCentavos > est.UsoMes.LimiteCentavos
}

// PrazoSaldoWhoLead limita a consulta da conta na estimativa.
var PrazoSaldoWhoLead = 8 * time.Second

// EstimarEnriquecimento conta a seleção, calcula o custo e, com conta real,
// consulta o saldo (falha do saldo não impede a estimativa: saldo = null).
func EstimarEnriquecimento(ctx context.Context, db *sql.DB, listaID int64, sel models.ProspSelecao, reprocessar bool, comSaldo bool) (*EnriquecerEstimativa, error) {
	resumo, err := models.ResumoProspSelecao(db, listaID, sel)
	if err != nil {
		return nil, err
	}
	cfg := models.LoadEnrichmentSettings(db)
	est := &EnriquecerEstimativa{Selecionados: resumo.Total, JaEnriquecidos: resumo.PorEstado[models.ItemEnriquecido], cfg: cfg}
	for _, estado := range enriqEstados(reprocessar) {
		est.AEnriquecer += resumo.PorEstado[estado]
	}
	est.Provedor = ProvedorEstimativa{Codigo: models.ProvedorWhoLead, Nome: models.NomeProvedor(models.ProvedorWhoLead),
		Pronto: cfg.WhoLeadPronto(), Demo: cfg.WhoLead.Demo, Motivo: cfg.WhoLeadMotivo()}
	est.CustoUnitarioCentavos = enriquecimento.NovoRegistro(cfg).CustoEstimado(models.ProvedorWhoLead, nil)
	est.CustoEstimadoCentavos = est.CustoUnitarioCentavos * est.AEnriquecer
	uso, err := models.EnrichmentUsage(db, time.Now())
	if err != nil {
		return nil, err
	}
	est.UsoMes = UsoMesEstimativa{CustoCentavos: uso.CustoCentavos, LimiteCentavos: cfg.LimiteMensalCentavos}
	if comSaldo && est.Provedor.Pronto && !est.Provedor.Demo {
		if w, err := enriquecimento.NovoRegistro(cfg).WhoLead(); err == nil {
			c, cancelar := context.WithTimeout(ctx, PrazoSaldoWhoLead)
			conta, err := w.Conta(c)
			cancelar()
			if err == nil {
				est.Saldo = &SaldoEstimativa{Plano: conta.Plano, FranquiaTotal: conta.FranquiaTotal, FranquiaRestante: conta.FranquiaRestante}
			}
		}
	}
	return est, nil
}

// CriarJobEnriquecer insere o job com os parâmetros da estimativa
// (models.ErrJobAtivo quando a lista já tem um processamento).
func CriarJobEnriquecer(db *sql.DB, listaID int64, est *EnriquecerEstimativa, sel models.ProspSelecao, reprocessar bool, userID *int64) (*models.ProspJob, error) {
	par := ParametrosEnriquecer{
		Selecao: sel, Reprocessar: reprocessar, Demo: est.cfg.WhoLead.Demo, LoteTamanho: est.cfg.WhoLead.LoteTamanho,
		CustoUnitarioCentavos: est.CustoUnitarioCentavos, CustoEstimadoCentavos: est.CustoEstimadoCentavos,
	}
	b, err := json.Marshal(par)
	if err != nil {
		return nil, err
	}
	j := &models.ProspJob{Tipo: models.JobEnriquecer, ListaID: &listaID, Parametros: b, Total: est.AEnriquecer,
		CriadoPor: userID, Mensagem: "na fila para enriquecer"}
	if err := models.CreateProspJob(db, j); err != nil {
		return nil, err
	}
	return j, nil
}
