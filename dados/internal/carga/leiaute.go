package carga

import (
	"strings"

	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

// Estatisticas resume a leitura de um arquivo.
type Estatisticas struct {
	Lidas      int64        `json:"lidas"`      // registros lidos do CSV
	Gravadas   int64        `json:"gravadas"`   // linhas enviadas à tabela de carga
	NULs       int64        `json:"nuls"`       // bytes 0x00 removidos do fluxo
	Invalidas  int64        `json:"invalidas"`  // registros sem chave utilizável
	Fantasmas  int64        `json:"fantasmas"`  // Empresas com razão vazia e natureza 0000
	Menores    int64        `json:"menores"`    // sócios de faixa etária 1 e 2, descartados
	CortesCPF  texto.Cortes `json:"cortes_cpf"` // CPFs removidos de nomes, por padrão
	Rejeitadas int64        `json:"rejeitadas"` // linhas da PGFN fora do padrão
}

func (e *Estatisticas) Somar(o Estatisticas) {
	e.Lidas += o.Lidas
	e.Gravadas += o.Gravadas
	e.NULs += o.NULs
	e.Invalidas += o.Invalidas
	e.Fantasmas += o.Fantasmas
	e.Menores += o.Menores
	e.Rejeitadas += o.Rejeitadas
	e.CortesCPF.Somar(o.CortesCPF)
}

// Leiaute descreve um arquivo da Receita: quantos campos tem o CSV, em que
// tabela de carga entra e como cada registro é limpo. Linha devolve nil para
// pular o registro (estabelecimento inativo, fora do recorte de UFs, sócio
// menor de idade...). Todas as colunas de carga são text; vazio vira NULL.
type Leiaute struct {
	Tabela  string
	Campos  int
	Colunas []string
	Linha   func(r []string, e *Estatisticas, ufs map[string]bool) []any
}

func nulo(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func basico(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if len(v) != 8 {
		return ""
	}
	for i := 0; i < 8; i++ {
		if !(v[i] >= '0' && v[i] <= '9') && !(v[i] >= 'A' && v[i] <= 'Z') {
			return ""
		}
	}
	return v
}

func nome(v string, e *Estatisticas) string {
	limpo, cortes := texto.RemoveCPF(texto.Limpa(v))
	e.CortesCPF.Somar(cortes)
	return limpo
}

// Leiautes dos arquivos da Receita, pela ordem oficial das colunas.
var (
	// Empresas: cnpj_basico, razao_social, natureza_juridica,
	// qualificacao_responsavel, capital_social, porte_empresa, ente_federativo.
	LeiauteEmpresas = Leiaute{
		Tabela: "empresas", Campos: 7,
		Colunas: []string{"cnpj_basico", "razao_social", "razao_busca", "natureza", "capital", "porte"},
		Linha: func(r []string, e *Estatisticas, _ map[string]bool) []any {
			b := basico(r[0])
			if b == "" {
				e.Invalidas++
				return nil
			}
			natureza := texto.Codigo(r[2], 4)
			if strings.TrimSpace(r[1]) == "" && (natureza == "0000" || natureza == "") {
				e.Fantasmas++ // linha fantasma que repete um cnpj_basico
				return nil
			}
			razao := nome(r[1], e)
			return []any{b, nulo(razao), nulo(texto.SemAcento(razao)), nulo(natureza), nulo(texto.Capital(r[4])), nulo(texto.Codigo(r[5], 2))}
		},
	}

	// Estabelecimentos (30 campos): só os ativos (situação 02) entram.
	LeiauteEstabelecimentos = Leiaute{
		Tabela: "estabelecimentos", Campos: 30,
		Colunas: []string{"cnpj", "cnpj_basico", "matriz", "nome_fantasia", "fantasia_busca", "data_situacao", "data_inicio",
			"cnae", "cnaes_sec", "tipo_logradouro", "logradouro", "numero", "complemento", "bairro", "bairro_busca", "cep",
			"uf", "municipio", "ddd1", "tel1", "ddd2", "tel2", "fone1", "tipo1", "fone2", "tipo2", "email", "email_tipo"},
		Linha: func(r []string, e *Estatisticas, ufs map[string]bool) []any {
			if strings.TrimSpace(r[5]) != "02" {
				return nil
			}
			uf := strings.ToUpper(strings.TrimSpace(r[19]))
			if len(uf) != 2 {
				uf = "EX"
			}
			if len(ufs) > 0 && !ufs[uf] {
				return nil
			}
			b := basico(r[0])
			cnpj := texto.NormalizarCNPJ(r[0] + r[1] + r[2])
			if b == "" || !texto.CNPJFormaValida(cnpj) {
				e.Invalidas++
				return nil
			}
			matriz := "0"
			if strings.TrimSpace(r[3]) == "1" {
				matriz = "1"
			}
			fantasia := nome(r[4], e)
			cnae := texto.Codigo(r[11], 7)
			bairro := texto.Limpa(r[17])
			cep := texto.SoDigitos(r[18])
			switch len(cep) {
			case 8:
			case 7:
				cep = "0" + cep
			default:
				cep = ""
			}
			f1, t1 := texto.Telefone(r[21], r[22])
			f2, t2 := texto.Telefone(r[23], r[24])
			if f2 == f1 {
				f2, t2 = "", ""
			}
			if f1 == "" && f2 != "" { // o primeiro telefone é sempre o melhor disponível
				f1, t1, f2, t2 = f2, t2, "", ""
			}
			email := texto.Email(r[27])
			return []any{cnpj, b, matriz, nulo(fantasia), nulo(texto.SemAcento(fantasia)), nulo(texto.Data(r[6])), nulo(texto.Data(r[10])),
				nulo(cnae), nulo(secundarios(r[12], cnae)), nulo(texto.Limpa(r[13])), nulo(texto.Limpa(r[14])), nulo(texto.Limpa(r[15])),
				nulo(texto.Limpa(r[16])), nulo(bairro), nulo(texto.SemAcento(bairro)), nulo(cep), uf, nulo(texto.Codigo(r[20], 4)),
				nulo(texto.SoDigitos(r[21])), nulo(texto.SoDigitos(r[22])), nulo(texto.SoDigitos(r[23])), nulo(texto.SoDigitos(r[24])),
				nulo(f1), nulo(t1), nulo(f2), nulo(t2), nulo(email), nulo(texto.TipoEmail(email))}
		},
	}

	// Simples: cnpj_basico, opcao_simples, data_opcao, data_exclusao,
	// opcao_mei, data_opcao_mei, data_exclusao_mei. Só optantes entram.
	LeiauteSimples = Leiaute{
		Tabela: "simples", Campos: 7,
		Colunas: []string{"cnpj_basico", "simples", "mei"},
		Linha: func(r []string, e *Estatisticas, _ map[string]bool) []any {
			b := basico(r[0])
			if b == "" {
				e.Invalidas++
				return nil
			}
			s, m := strings.TrimSpace(r[1]) == "S", strings.TrimSpace(r[4]) == "S"
			if !s && !m {
				return nil
			}
			return []any{b, sn(s), sn(m)}
		},
	}

	// Sócios: cnpj_basico, identificador (1 PJ, 2 PF, 3 estrangeiro), nome,
	// documento, qualificacao, data_entrada, pais, cpf do representante, nome
	// do representante, qualificacao do representante, faixa_etaria.
	LeiauteSocios = Leiaute{
		Tabela: "socios", Campos: 11,
		Colunas: []string{"cnpj_basico", "tipo", "nome", "documento", "qualificacao", "desde", "representante"},
		Linha: func(r []string, e *Estatisticas, _ map[string]bool) []any {
			b := basico(r[0])
			if b == "" {
				e.Invalidas++
				return nil
			}
			if f := strings.TrimSpace(r[10]); f == "1" || f == "2" {
				e.Menores++ // menores de idade nunca entram na base
				return nil
			}
			tipo := "pf"
			switch strings.TrimSpace(r[1]) {
			case "1":
				tipo = "pj"
			case "3":
				tipo = "estrangeiro"
			}
			doc := texto.Limpa(r[3])
			if tipo != "pj" {
				doc = texto.MascaraCPF(doc) // já vem mascarado; garante que segue assim
			}
			return []any{b, tipo, nulo(nome(r[2], e)), nulo(doc), nulo(texto.Codigo(r[4], 2)), nulo(texto.Data(r[5])), nulo(nome(r[8], e))}
		},
	}

	LeiauteCnaes = Leiaute{
		Tabela: "cnaes", Campos: 2, Colunas: []string{"codigo", "descricao", "descricao_busca"},
		Linha: func(r []string, e *Estatisticas, _ map[string]bool) []any {
			c, d := texto.Codigo(r[0], 7), texto.Limpa(r[1])
			if c == "" {
				e.Invalidas++
				return nil
			}
			return []any{c, d, texto.SemAcento(d)}
		},
	}
	LeiauteMunicipios = Leiaute{
		Tabela: "municipios", Campos: 2, Colunas: []string{"codigo", "nome", "nome_busca"},
		Linha: func(r []string, e *Estatisticas, _ map[string]bool) []any {
			c, d := texto.Codigo(r[0], 4), texto.Limpa(r[1])
			if c == "" {
				e.Invalidas++
				return nil
			}
			return []any{c, d, texto.SemAcento(d)}
		},
	}
	LeiauteNaturezas     = dominio("naturezas", 4)
	LeiauteQualificacoes = dominio("qualificacoes", 2)
)

func dominio(tabela string, digitos int) Leiaute {
	return Leiaute{
		Tabela: tabela, Campos: 2, Colunas: []string{"codigo", "descricao"},
		Linha: func(r []string, e *Estatisticas, _ map[string]bool) []any {
			c := texto.Codigo(r[0], digitos)
			if c == "" {
				e.Invalidas++
				return nil
			}
			return []any{c, texto.Limpa(r[1])}
		},
	}
}

func sn(v bool) string {
	if v {
		return "S"
	}
	return "N"
}

// secundarios valida a lista de CNAEs secundários ("4711302,4721102"),
// tirando repetidos e o principal.
func secundarios(v, principal string) string {
	if strings.TrimSpace(v) == "" {
		return ""
	}
	vistos := map[string]bool{principal: true}
	saida := make([]string, 0, 4)
	for _, p := range strings.Split(v, ",") {
		c := texto.Codigo(p, 7)
		if c == "" || vistos[c] {
			continue
		}
		vistos[c] = true
		saida = append(saida, c)
	}
	return strings.Join(saida, ",")
}

// leiautes na ordem em que as tabelas de carga são criadas.
var leiautes = []Leiaute{LeiauteEmpresas, LeiauteEstabelecimentos, LeiauteSimples, LeiauteSocios,
	LeiauteCnaes, LeiauteMunicipios, LeiauteNaturezas, LeiauteQualificacoes}

// ArquivosCNPJ são os 37 nomes (sem .zip) que tornam uma referência
// completa.
func ArquivosCNPJ() []string {
	nomes := []string{}
	for _, p := range []string{"Empresas", "Estabelecimentos", "Socios"} {
		for i := 0; i < 10; i++ {
			nomes = append(nomes, p+string(rune('0'+i)))
		}
	}
	return append(nomes, "Simples", "Cnaes", "Motivos", "Municipios", "Naturezas", "Paises", "Qualificacoes")
}

// LeiauteDoArquivo devolve o leiaute pelo nome do zip. Paises e Motivos não
// são carregados (a base só tem estabelecimentos ativos no Brasil): ok=false.
func LeiauteDoArquivo(nome string) (Leiaute, bool) {
	n := strings.TrimSuffix(nome, ".zip")
	switch {
	case strings.HasPrefix(n, "Empresas"):
		return LeiauteEmpresas, true
	case strings.HasPrefix(n, "Estabelecimentos"):
		return LeiauteEstabelecimentos, true
	case strings.HasPrefix(n, "Socios"):
		return LeiauteSocios, true
	case n == "Simples":
		return LeiauteSimples, true
	case n == "Cnaes":
		return LeiauteCnaes, true
	case n == "Municipios":
		return LeiauteMunicipios, true
	case n == "Naturezas":
		return LeiauteNaturezas, true
	case n == "Qualificacoes":
		return LeiauteQualificacoes, true
	}
	return Leiaute{}, false
}
