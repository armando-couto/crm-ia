package enriquecimento

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
)

// Demo devolve um perfil fictício e determinístico (função do CNPJ) para
// apresentar o recurso antes de haver contrato com o provedor pago. Sempre
// rotulado "dados de demonstração" e sem custo.
type Demo struct {
	Fonte          string
	Precos         map[string]int
	DatasetsPadrao []string
	PrecoFixo      int
}

func (d *Demo) Codigo() string { return d.Fonte }

// sorteio é um gerador determinístico a partir do CNPJ + fonte.
type sorteio struct{ estado uint64 }

func novoSorteio(semente string) *sorteio {
	h := fnv.New64a()
	_, _ = h.Write([]byte(semente))
	return &sorteio{estado: h.Sum64() | 1}
}

// ate devolve um inteiro em [0, n).
func (s *sorteio) ate(n int) int {
	if n <= 0 {
		return 0
	}
	// xorshift64*
	s.estado ^= s.estado >> 12
	s.estado ^= s.estado << 25
	s.estado ^= s.estado >> 27
	return int((s.estado * 2685821657736338717) >> 33 % uint64(n))
}

func (s *sorteio) escolher(opcoes ...string) string { return opcoes[s.ate(len(opcoes))] }

var (
	faixasFuncionarios = []string{"1 a 9", "10 a 49", "50 a 99", "100 a 499", "500 ou mais"}
	faixasFaturamento  = []string{"até R$ 360 mil", "R$ 360 mil a 4,8 M", "R$ 4,8 M a 10 M", "R$ 10 M a 50 M", "acima de R$ 50 M"}
)

func (d *Demo) Consultar(ctx context.Context, cnpj string, opts Opcoes) (*Perfil, *Bruto, error) {
	digits := SomenteDigitos(cnpj)
	if !ValidarCNPJ(digits) {
		return nil, nil, falha(FalhaEntrada, "CNPJ inválido")
	}
	s := novoSorteio(d.Fonte + ":" + digits)
	p := NovoPerfil(d.Fonte)
	p.Demo = true
	p.Mensagem = "dados de demonstração"
	p.CustoCentavos = 0
	p.Set("cnpj", FormatarCNPJ(digits))

	faixa := s.ate(5)
	switch d.Fonte {
	case models.ProvedorBigDataCorp:
		ind := &Indicadores{
			NivelAtividade:      float64(35+s.ate(60)) / 100,
			TemDominioAtivo:     s.ate(10) < 8,
			TemSSL:              s.ate(10) < 7,
			TemEmailCorporativo: s.ate(10) < 7,
			FuncionariosFaixa:   faixasFuncionarios[faixa],
			FaturamentoFaixa:    faixasFaturamento[faixa],
		}
		if !ind.TemDominioAtivo {
			ind.TemSSL = false
		}
		p.Indicadores = ind
		p.Set("employees_range", ind.FuncionariosFaixa)
		p.Set("revenue_range", ind.FaturamentoFaixa)
		ddd := 11 + s.ate(80)
		p.Contatos = &Contatos{
			Telefones: []Telefone{
				{Numero: fmt.Sprintf("+55%d9%08d", ddd, s.ate(100000000)), Tipo: "mobile", Principal: true, Validado: true},
				{Numero: fmt.Sprintf("+55%d3%07d", ddd, s.ate(10000000)), Tipo: "fixo", Principal: false, Validado: s.ate(2) == 0},
			},
			Emails: []EmailContato{
				{Email: "contato@empresa-demo.com.br", Tipo: "corporativo", Validado: true},
				{Email: "financeiro@empresa-demo.com.br", Tipo: "corporativo", Validado: s.ate(2) == 0},
			},
		}
		if ind.TemDominioAtivo {
			p.PresencaDigital = &PresencaDigital{
				Dominios: []Dominio{{Dominio: "empresa-demo.com.br", Ativo: true, SSL: ind.TemSSL}},
				Redes:    map[string]string{"instagram": "@empresademo", "linkedin": "company/empresa-demo"},
			}
		}
		total := s.ate(3)
		pr := &Processos{Ultimos: []Processo{}}
		for i := 0; i < total; i++ {
			valor := float64(5000 + s.ate(60000))
			polo := s.escolher("Réu", "Autor")
			status := s.escolher("Ativo", "Arquivado")
			if polo == "Réu" {
				pr.ComoReu++
			}
			if status == "Ativo" {
				pr.Ativos++
			}
			pr.ValorTotal += valor
			pr.Ultimos = append(pr.Ultimos, Processo{
				Numero:   fmt.Sprintf("%07d-%02d.%d.8.06.%04d", s.ate(10000000), s.ate(100), 2019+s.ate(7), s.ate(10000)),
				Tipo:     s.escolher("Cível", "Trabalhista", "Execução fiscal"),
				Tribunal: s.escolher("TJCE", "TJSP", "TRT7", "TJMG"),
				Status:   status, Polo: polo, Valor: valor,
				Data: fmt.Sprintf("%d-%02d-%02d", 2020+s.ate(6), 1+s.ate(12), 1+s.ate(28)),
			})
		}
		pr.Total = total
		p.Processos = pr
		p.Conformidade = &Conformidade{PEP: s.ate(20) == 0, Sancionado: false, Listas: []string{}}
		p.Midia = &Midia{Exposicao: s.escolher("nenhuma", "baixa", "baixa", "media"), Noticias: []Noticia{}}
	case models.ProvedorSerasa:
		score := 300 + s.ate(651)
		r := &Risco{
			Score:        score,
			Faixa:        FaixaDoScore(score),
			Nivel:        NivelDoScore(score),
			Consultas90d: s.ate(13),
			Observacao:   "dados de demonstração",
		}
		if score < 650 {
			r.PendenciasQtd = s.ate(4)
			if r.PendenciasQtd > 0 {
				r.PendenciasValor = float64(r.PendenciasQtd) * float64(800+s.ate(9000))
			}
			r.ProtestosQtd = s.ate(3)
			r.ChequesSemFundo = s.ate(2)
			r.AcoesQtd = s.ate(3)
		}
		r.LimiteSugerido = float64((score/50)*(faixa+1)) * 1000
		if r.PendenciasQtd+r.ProtestosQtd > 2 {
			r.LimiteSugerido = r.LimiteSugerido / 4
		}
		p.Risco = r
		p.Socios = []Socio{{Nome: "SÓCIO DEMONSTRAÇÃO", Documento: "***000000**", Qualificacao: "Sócio-Administrador · 100%", Desde: "2015-03-10", Tipo: "PF"}}
	default:
		return nil, nil, falha(FalhaEntrada, "fonte desconhecida: %s", d.Fonte)
	}
	p.Sinais = CalcularSinais(p)
	bruto, _ := json.Marshal(map[string]any{"demo": true, "fonte": d.Fonte, "cnpj": digits,
		"datasets": strings.Join(opts.Datasets, ",")})
	return p, &Bruto{Fonte: d.Fonte + "-demo", Corpo: json.RawMessage(bruto)}, nil
}
