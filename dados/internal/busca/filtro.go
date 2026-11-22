package busca

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

// sqlb junta os argumentos posicionais de uma consulta.
type sqlb struct{ args []any }

func (b *sqlb) p(v any) string {
	b.args = append(b.args, v)
	return "$" + strconv.Itoa(len(b.args))
}

type sinal struct {
	coluna string
	valor  bool
}

// plano é o filtro já validado e separado por onde pode ser respondido: as
// colunas dos cubos (lugar, CNAE e sinais) e o resto (extras), que só a
// tabela de busca responde.
type plano struct {
	vazio bool // o filtro não casa com nada (prefixo de CNAE sem subclasse...)

	cnpjs []string

	ufs        []string
	municipios []int16

	filtraCnae      bool
	cnaes           []int32
	qualquerPosicao bool

	portes []int16
	sinais []sinal

	extras []func(b *sqlb) string

	faixa    faixa
	temFaixa bool
}

var (
	reUF        = regexp.MustCompile(`^[A-Z]{2}$`)
	reDataISO   = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	codigoPorte = map[string]int16{"me": 1, "epp": 3, "demais": 5, "nao_informado": 0}
)

// NomePorte converte o código da Receita (01, 03, 05, 00) no valor do
// contrato.
func NomePorte(codigo int) string {
	switch codigo {
	case 1:
		return "me"
	case 3:
		return "epp"
	case 5:
		return "demais"
	}
	return "nao_informado"
}

func tri(campo, v string, sim, nao string) (filtra, valor bool, err error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "todos", "todas":
		return false, false, nil
	case sim:
		return true, true, nil
	case nao:
		return true, false, nil
	}
	return false, false, invalido(fmt.Sprintf("valor inválido em %s: %q", campo, v))
}

func dataFiltro(campo, v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if !reDataISO.MatchString(v) {
		return "", invalido(campo + " deve ser uma data AAAA-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return "", invalido(campo + " não é uma data válida")
	}
	return v, nil
}

func centavos(campo string, v *float64) (int64, bool, error) {
	if v == nil {
		return 0, false, nil
	}
	if *v < 0 || *v > 9e15 {
		return 0, false, invalido(campo + " fora do intervalo aceito")
	}
	return int64(*v*100 + 0.5), true, nil
}

// planejar valida os filtros e os traduz. Regras do contrato: é obrigatório
// ao menos um entre ufs, municipios, cnaes, cnae_prefixos e cnpjs; campo
// ausente, vazio ou "todos" não filtra; todo filtro vale para o
// estabelecimento (CNPJ de 14); com cnpjs os demais filtros são ignorados,
// exceto incluir_pf.
func (c *catalogo) planejar(f Filtros) (*plano, error) {
	p := &plano{}
	if !f.IncluirPF {
		// pessoa física (MEI, empresário individual, naturezas 4000 a 4999)
		// fica fora por padrão
		p.sinais = append(p.sinais, sinal{"pf", false})
	}

	if len(f.CNPJs) > 0 {
		if len(f.CNPJs) > MaximoCNPJs {
			return nil, invalido(fmt.Sprintf("cnpjs aceita até %d itens", MaximoCNPJs))
		}
		vistos := map[string]bool{}
		for _, v := range f.CNPJs {
			n := texto.NormalizarCNPJ(v)
			if texto.CNPJFormaValida(n) && !vistos[n] {
				vistos[n] = true
				p.cnpjs = append(p.cnpjs, n)
			}
		}
		if len(p.cnpjs) == 0 {
			p.vazio = true
		}
		return p, nil
	}

	if len(f.UFs) == 0 && len(f.Municipios) == 0 && len(f.Cnaes) == 0 && len(f.CnaePrefixos) == 0 {
		return nil, invalido("informe ao menos um filtro entre ufs, municipios, cnaes, cnae_prefixos e cnpjs")
	}
	if len(f.UFs) > 30 || len(f.Municipios) > 1000 || len(f.Cnaes) > 1500 || len(f.CnaePrefixos) > 100 || len(f.Naturezas) > 200 || len(f.Bairros) > 200 || len(f.CEPs) > 500 {
		return nil, invalido("lista de filtro grande demais")
	}

	// lugar
	ufs := map[string]bool{}
	for _, v := range f.UFs {
		uf := strings.ToUpper(strings.TrimSpace(v))
		if !reUF.MatchString(uf) {
			return nil, invalido(fmt.Sprintf("UF inválida: %q", v))
		}
		ufs[uf] = true
	}
	for _, m := range f.Municipios {
		if m < 1 || m > 9999 {
			return nil, invalido(fmt.Sprintf("município inválido: %d (use o código da Receita)", m))
		}
		p.municipios = append(p.municipios, int16(m))
	}
	if len(p.municipios) > 0 {
		// a UF vai sempre junto do município (é a primeira coluna do índice);
		// com ufs informadas, vale a interseção
		ufsDosMunicipios := map[string]bool{}
		var validos []int16
		p.temFaixa = true
		p.faixa = faixa{min: 1<<31 - 1, max: 0}
		for _, m := range p.municipios {
			mun := c.munPorCod[int(m)]
			fx, ok := c.faixaMun[int(m)]
			if mun == nil || !ok || (len(ufs) > 0 && !ufs[mun.UF]) {
				continue
			}
			ufsDosMunicipios[mun.UF] = true
			validos = append(validos, m)
			p.faixa.min, p.faixa.max = min(p.faixa.min, fx.min), max(p.faixa.max, fx.max)
		}
		p.municipios, ufs = validos, ufsDosMunicipios
		if len(validos) == 0 {
			p.vazio = true
		}
	} else if len(ufs) > 0 {
		p.temFaixa = true
		p.faixa = faixa{min: 1<<31 - 1, max: 0}
		algum := false
		for uf := range ufs {
			if fx, ok := c.faixaUF[uf]; ok {
				algum = true
				p.faixa.min, p.faixa.max = min(p.faixa.min, fx.min), max(p.faixa.max, fx.max)
			}
		}
		if !algum {
			p.vazio = true
		}
	}
	for uf := range ufs {
		p.ufs = append(p.ufs, uf)
	}
	sort.Strings(p.ufs)

	// CNAE: códigos exatos e prefixos (2 a 5 dígitos) expandidos pelo catálogo
	if len(f.Cnaes) > 0 || len(f.CnaePrefixos) > 0 {
		p.filtraCnae, p.qualquerPosicao = true, f.CnaeSecundario
		codigos := map[int32]bool{}
		for _, v := range f.Cnaes {
			if v < 1 || v > 9999999 {
				return nil, invalido(fmt.Sprintf("CNAE inválido: %d (use a subclasse de 7 dígitos)", v))
			}
			codigos[int32(v)] = true
		}
		for _, v := range f.CnaePrefixos {
			pre := texto.SoDigitos(v)
			if len(pre) < 2 || len(pre) > 7 {
				return nil, invalido(fmt.Sprintf("prefixo de CNAE inválido: %q (2 a 5 dígitos)", v))
			}
			for _, cod := range c.subclasses {
				if strings.HasPrefix(fmt.Sprintf("%07d", cod), pre) {
					codigos[int32(cod)] = true
				}
			}
		}
		for cod := range codigos {
			p.cnaes = append(p.cnaes, cod)
		}
		sort.Slice(p.cnaes, func(i, j int) bool { return p.cnaes[i] < p.cnaes[j] })
		if len(p.cnaes) == 0 {
			p.vazio = true
		}
	}

	// sinais (colunas dos cubos)
	if len(f.Portes) > 0 {
		vistos := map[int16]bool{}
		for _, v := range f.Portes {
			cod, ok := codigoPorte[strings.ToLower(strings.TrimSpace(v))]
			if !ok {
				return nil, invalido(fmt.Sprintf("porte inválido: %q", v))
			}
			if !vistos[cod] {
				vistos[cod] = true
				p.portes = append(p.portes, cod)
			}
		}
	}
	if filtra, v, err := tri("matriz", f.Matriz, "matriz", "filial"); err != nil {
		return nil, err
	} else if filtra {
		p.sinais = append(p.sinais, sinal{"matriz", v})
	}
	if filtra, v, err := tri("simples", f.Simples, "sim", "nao"); err != nil {
		return nil, err
	} else if filtra {
		p.sinais = append(p.sinais, sinal{"simples", v})
	}
	if filtra, v, err := tri("mei", f.MEI, "sim", "nao"); err != nil {
		return nil, err
	} else if filtra {
		if v && !f.IncluirPF {
			// todo MEI é pessoa física: sem a chave o resultado seria sempre zero
			return nil, invalido("para filtrar MEI, ligue 'incluir MEI e empresário individual'")
		}
		p.sinais = append(p.sinais, sinal{"mei", v})
	}
	if f.ComTelefone {
		p.sinais = append(p.sinais, sinal{"tem_fone", true}) // qualquer telefone válido
	}
	if f.ComCelular {
		p.sinais = append(p.sinais, sinal{"tem_celular", true})
	}
	if f.ComEmail {
		p.sinais = append(p.sinais, sinal{"tem_email", true})
	}
	if filtra, v, err := tri("divida", f.Divida, "com", "sem"); err != nil {
		return nil, err
	} else if filtra {
		p.sinais = append(p.sinais, sinal{"tem_divida", v})
	}

	// extras (fora dos cubos)
	if len(f.Naturezas) > 0 {
		var nats []int16
		for _, n := range f.Naturezas {
			if n < 0 || n > 9999 {
				return nil, invalido(fmt.Sprintf("natureza jurídica inválida: %d", n))
			}
			nats = append(nats, int16(n))
		}
		p.extras = append(p.extras, func(b *sqlb) string { return "b.natureza = ANY(" + b.p(nats) + ")" })
	}
	de, err := dataFiltro("abertura_de", f.AberturaDe)
	if err != nil {
		return nil, err
	}
	ate, err := dataFiltro("abertura_ate", f.AberturaAte)
	if err != nil {
		return nil, err
	}
	if f.AberturaUltimosDias < 0 || f.AberturaUltimosDias > 36500 {
		return nil, invalido("abertura_ultimos_dias inválido")
	}
	if f.AberturaUltimosDias > 0 {
		if d := time.Now().AddDate(0, 0, -f.AberturaUltimosDias).Format("2006-01-02"); d > de {
			de = d
		}
	}
	if de != "" {
		p.extras = append(p.extras, func(b *sqlb) string { return "b.data_inicio >= " + b.p(de) + "::date" })
	}
	if ate != "" {
		p.extras = append(p.extras, func(b *sqlb) string { return "b.data_inicio <= " + b.p(ate) + "::date" })
	}
	for _, lim := range []struct {
		campo, coluna, op string
		v                 *float64
	}{{"capital_min", "b.capital", ">=", f.CapitalMin}, {"capital_max", "b.capital", "<=", f.CapitalMax},
		{"divida_min", "b.divida", ">=", f.DividaMin}, {"divida_max", "b.divida", "<=", f.DividaMax}} {
		c, tem, err := centavos(lim.campo, lim.v)
		if err != nil {
			return nil, err
		}
		if tem {
			coluna, op := lim.coluna, lim.op
			p.extras = append(p.extras, func(b *sqlb) string { return coluna + " " + op + " " + b.p(c) })
		}
	}
	if f.OcultarContador {
		p.extras = append(p.extras, func(*sqlb) string { return "NOT b.contador" })
	}

	// nome, bairros e ceps não têm índice próprio: são conferidos na ficha
	// sobre o resultado dos demais, por isso exigem município ou CNAE
	var naFicha []func(b *sqlb) string
	if nome := texto.SemAcento(f.Nome); nome != "" {
		letras := 0
		for _, r := range nome {
			if r != ' ' {
				letras++
			}
		}
		if letras < 3 {
			return nil, invalido("nome precisa de ao menos 3 letras")
		}
		padrao := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(nome) + "%"
		naFicha = append(naFicha, func(b *sqlb) string { return "f.nome_busca LIKE " + b.p(padrao) })
	}
	if len(f.Bairros) > 0 {
		var bairros []string
		for _, v := range f.Bairros {
			if n := texto.SemAcento(v); n != "" {
				bairros = append(bairros, n)
			}
		}
		if len(bairros) > 0 {
			naFicha = append(naFicha, func(b *sqlb) string { return "f.bairro_busca = ANY(" + b.p(bairros) + ")" })
		}
	}
	if len(f.CEPs) > 0 {
		var inteiros, prefixos []string
		for _, v := range f.CEPs {
			d := texto.SoDigitos(v)
			switch {
			case len(d) == 8:
				inteiros = append(inteiros, d)
			case len(d) >= 3 && len(d) < 8:
				prefixos = append(prefixos, d+"%")
			default:
				return nil, invalido(fmt.Sprintf("CEP inválido: %q (8 dígitos ou prefixo de ao menos 3)", v))
			}
		}
		naFicha = append(naFicha, func(b *sqlb) string {
			var ou []string
			if len(inteiros) > 0 {
				ou = append(ou, "f.cep = ANY("+b.p(inteiros)+")")
			}
			if len(prefixos) > 0 {
				ou = append(ou, "f.cep LIKE ANY("+b.p(prefixos)+")")
			}
			return "(" + strings.Join(ou, " OR ") + ")"
		})
	}
	if len(naFicha) > 0 {
		if len(p.municipios) == 0 && !p.filtraCnae && !p.vazio {
			return nil, invalido("nome, bairros e ceps exigem municipios, cnaes ou cnae_prefixos no mesmo filtro")
		}
		p.extras = append(p.extras, func(b *sqlb) string {
			partes := make([]string, 0, len(naFicha))
			for _, c := range naFicha {
				partes = append(partes, c(b))
			}
			return "EXISTS (SELECT 1 FROM {E}.base_ativa f WHERE f.id = b.id AND " + strings.Join(partes, " AND ") + ")"
		})
	}
	return p, nil
}

// Alvos de uma condição: cada tabela conhece um subconjunto das colunas.
const (
	alvoBusca      = iota // tudo
	alvoCuboMun           // lugar + sinais
	alvoCuboUFCnae        // uf + CNAE principal + sinais
	alvoCuboQQ            // uf + CNAE em qualquer posição + sinais
)

// onde monta o WHERE do plano para o alvo, com o alias b.
func (p *plano) onde(b *sqlb, alvo int) string {
	conds := []string{"true"}
	if len(p.cnpjs) > 0 {
		conds = append(conds, "b.id IN (SELECT id FROM {E}.base_ativa WHERE cnpj = ANY("+b.p(p.cnpjs)+"))")
	}
	if len(p.ufs) > 0 {
		conds = append(conds, "b.uf = ANY("+b.p(p.ufs)+")")
	}
	if len(p.municipios) > 0 && (alvo == alvoBusca || alvo == alvoCuboMun) {
		conds = append(conds, "b.municipio = ANY("+b.p(p.municipios)+")")
	}
	if p.filtraCnae {
		switch {
		case alvo == alvoCuboUFCnae || alvo == alvoCuboQQ || (alvo == alvoBusca && !p.qualquerPosicao):
			conds = append(conds, "b.cnae = ANY("+b.p(p.cnaes)+")")
		case alvo == alvoBusca:
			conds = append(conds, "b.cnaes && "+b.p(p.cnaes)+"::int[]")
		}
	}
	if len(p.portes) > 0 {
		conds = append(conds, "b.porte = ANY("+b.p(p.portes)+")")
	}
	for _, s := range p.sinais {
		if s.valor {
			conds = append(conds, "b."+s.coluna)
		} else {
			conds = append(conds, "NOT b."+s.coluna)
		}
	}
	if alvo == alvoBusca {
		for _, e := range p.extras {
			conds = append(conds, e(b))
		}
	}
	return strings.Join(conds[min(1, len(conds)-1):], " AND ")
}
