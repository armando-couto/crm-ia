package models

import (
	"errors"
	"testing"
	"time"
)

func TestConfigProspeccaoPadraoEValidacao(t *testing.T) {
	c := DefaultConfigProspeccao()
	if err := c.Validate(); err != nil {
		t.Fatalf("o padrão deveria ser válido: %v", err)
	}
	if c.JanelaInicio != 9 || c.JanelaFim != 18 || c.MaxTentativas != 6 || c.TentativasDia != 2 ||
		c.RetornoPadraoHoras != 48 || c.MetaLigacoesDia != 40 || !c.ExigirOptinWhatsApp || c.IncluirSemDonoNaFila ||
		len(c.IntervalosHoras) != 5 || len(c.MotivosDescarte) != 5 {
		t.Fatalf("padrões fora do contrato: %+v", c)
	}

	invalidos := map[string]func(*ConfigProspeccao){
		"janela antes do teto":    func(c *ConfigProspeccao) { c.JanelaInicio = 7 },
		"janela depois do teto":   func(c *ConfigProspeccao) { c.JanelaFim = 21 },
		"janela invertida":        func(c *ConfigProspeccao) { c.JanelaInicio, c.JanelaFim = 18, 9 },
		"zero tentativas":         func(c *ConfigProspeccao) { c.MaxTentativas = 0 },
		"intervalo zero":          func(c *ConfigProspeccao) { c.IntervalosHoras = []int{3, 0} },
		"tentativas por dia":      func(c *ConfigProspeccao) { c.TentativasDia = 0 },
		"retorno zero":            func(c *ConfigProspeccao) { c.RetornoPadraoHoras = 0 },
		"meta negativa":           func(c *ConfigProspeccao) { c.MetaLigacoesDia = -1 },
		"sem motivos":             func(c *ConfigProspeccao) { c.MotivosDescarte = []string{"  "} },
		"motivo reservado":        func(c *ConfigProspeccao) { c.MotivosDescarte = []string{"opt_out"} },
		"etapa sem funil":         func(c *ConfigProspeccao) { id := int64(3); c.StageID = &id },
		"motivo longo demais":     func(c *ConfigProspeccao) { c.MotivosDescarte = []string{string(make([]rune, 81))} },
		"intervalos em excesso":   func(c *ConfigProspeccao) { c.IntervalosHoras = make([]int, 21) },
		"intervalo grande demais": func(c *ConfigProspeccao) { c.IntervalosHoras = []int{721} },
	}
	for nome, mexer := range invalidos {
		c := DefaultConfigProspeccao()
		mexer(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: esperava erro de validação", nome)
		}
	}

	// Motivos são aparados e deduplicados sem diferenciar caixa.
	c = DefaultConfigProspeccao()
	c.MotivosDescarte = []string{" Sem   interesse ", "sem interesse", "", "Fora do perfil"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(c.MotivosDescarte) != 2 || c.MotivosDescarte[0] != "Sem interesse" {
		t.Fatalf("motivos não normalizados: %q", c.MotivosDescarte)
	}
	if !c.MotivoDescarteValido("Fora do perfil") || c.MotivoDescarteValido("Outro") || c.MotivoDescarteValido("") {
		t.Error("MotivoDescarteValido deveria aceitar só os rótulos configurados")
	}
}

func TestIntervaloCadencia(t *testing.T) {
	c := DefaultConfigProspeccao() // [3, 24, 48, 72, 120]
	casos := map[int]int{1: 3, 2: 24, 5: 120, 6: 120, 9: 120, 0: 3}
	for tentativas, horas := range casos {
		if got := c.IntervaloCadencia(tentativas); got != time.Duration(horas)*time.Hour {
			t.Errorf("IntervaloCadencia(%d) = %s, quer %dh", tentativas, got, horas)
		}
	}
	c.IntervalosHoras = nil
	if got := c.IntervaloCadencia(3); got != 24*time.Hour {
		t.Errorf("lista vazia deveria valer 24h, veio %s", got)
	}
}

func TestJanelaDeContato(t *testing.T) {
	c := DefaultConfigProspeccao()
	em := func(ano int, mes time.Month, dia, hora, min int) time.Time {
		return time.Date(ano, mes, dia, hora, min, 0, 0, time.Local)
	}
	// 2026-11-23 é segunda-feira; 21 é sábado; 22 é domingo; 20 é feriado (Consciência Negra).
	seg := em(2026, 11, 23, 10, 30)
	if !c.JanelaAberta(seg) || !c.AjustarJanela(seg).Equal(seg) {
		t.Error("segunda 10h30 está dentro da janela e não deve ser deslocada")
	}
	if c.JanelaAberta(em(2026, 11, 23, 18, 0)) || c.JanelaAberta(em(2026, 11, 23, 8, 59)) {
		t.Error("18h00 e 8h59 ficam fora da janela 9h às 18h")
	}
	if got := c.AjustarJanela(em(2026, 11, 23, 7, 0)); !got.Equal(em(2026, 11, 23, 9, 0)) {
		t.Errorf("antes da abertura vai para as 9h do mesmo dia, veio %s", got)
	}
	if got := c.AjustarJanela(em(2026, 11, 23, 19, 0)); !got.Equal(em(2026, 11, 24, 9, 0)) {
		t.Errorf("depois do fim vai para as 9h do dia seguinte, veio %s", got)
	}
	// Quinta 19/11 à noite: sexta 20 é feriado, sábado e domingo não contam → segunda 23.
	if got := c.AjustarJanela(em(2026, 11, 19, 20, 0)); !got.Equal(em(2026, 11, 23, 9, 0)) {
		t.Errorf("feriado e fim de semana devem ser pulados, veio %s", got)
	}
	if c.DiaPermitido(em(2026, 11, 21, 10, 0)) || c.DiaPermitido(em(2026, 11, 22, 10, 0)) || c.DiaPermitido(em(2026, 11, 20, 10, 0)) {
		t.Error("sábado (desligado), domingo e feriado não são dias permitidos")
	}
	if got := c.ProximoDiaPermitido(em(2026, 11, 23, 10, 0)); !got.Equal(em(2026, 11, 24, 9, 0)) {
		t.Errorf("ProximoDiaPermitido deveria ir para o dia seguinte às 9h, veio %s", got)
	}

	c.Sabado = true
	if !c.JanelaAberta(em(2026, 11, 21, 10, 0)) {
		t.Error("com sábado ligado, sábado 10h está dentro da janela")
	}
	if c.JanelaAberta(em(2026, 11, 22, 10, 0)) {
		t.Error("domingo nunca")
	}
	if c.TextoJanela() != "segunda a sábado, 9h às 18h" {
		t.Errorf("texto da janela: %q", c.TextoJanela())
	}

	j := DefaultConfigProspeccao().JanelaEm(em(2026, 11, 22, 10, 0))
	if j.Aberta || j.ReabreEm == nil || !j.ReabreEm.Equal(em(2026, 11, 23, 9, 0)) || j.Texto != "segunda a sexta, 9h às 18h" {
		t.Errorf("janela no domingo: %+v", j)
	}
	if j := DefaultConfigProspeccao().JanelaEm(seg); !j.Aberta || j.ReabreEm != nil {
		t.Errorf("janela aberta não tem reabre_em: %+v", j)
	}
}

func TestFeriadoNacional(t *testing.T) {
	d := func(ano int, mes time.Month, dia int) time.Time {
		return time.Date(ano, mes, dia, 12, 0, 0, 0, time.Local)
	}
	feriados := []time.Time{
		d(2026, 1, 1), d(2026, 4, 3) /* Sexta-feira Santa */, d(2026, 4, 21), d(2026, 5, 1), d(2026, 9, 7),
		d(2026, 10, 12), d(2026, 11, 2), d(2026, 11, 15), d(2026, 11, 20), d(2026, 12, 25),
		d(2027, 3, 26) /* Sexta-feira Santa de 2027 */, d(2025, 4, 18),
	}
	for _, f := range feriados {
		if !FeriadoNacional(f) {
			t.Errorf("%s deveria ser feriado nacional", f.Format("02/01/2006"))
		}
	}
	for _, f := range []time.Time{d(2026, 2, 17) /* Carnaval */, d(2026, 6, 4) /* Corpus Christi */, d(2026, 11, 23), d(2023, 11, 20)} {
		if FeriadoNacional(f) {
			t.Errorf("%s não é feriado nacional", f.Format("02/01/2006"))
		}
	}
}

func TestMotivosPerda(t *testing.T) {
	m := DefaultMotivosPerda()
	if !m.Exigir || len(m.Motivos) != 7 || m.Motivos[0] != "Preço" || m.Motivos[6] != "Outro" {
		t.Fatalf("padrão fora do contrato: %+v", m)
	}
	if _, err := m.ValidarPerda(""); !errors.Is(err, ErrMotivoPerdaObrigatorio) {
		t.Errorf("com exigir, motivo vazio deveria dar ErrMotivoPerdaObrigatorio, veio %v", err)
	}
	if got, err := m.ValidarPerda("  Preço "); err != nil || got != "Preço" {
		t.Errorf("motivo da lista deveria passar aparado: %q, %v", got, err)
	}
	if _, err := m.ValidarPerda("Inventado"); err == nil {
		t.Error("motivo fora da lista deveria ser recusado")
	}
	m.Exigir = false
	if got, err := m.ValidarPerda(""); err != nil || got != "" {
		t.Errorf("sem exigir, motivo vazio passa: %q, %v", got, err)
	}
	vazio := MotivosPerda{Motivos: []string{" "}}
	if err := vazio.Validate(); err == nil {
		t.Error("lista sem motivos deveria ser recusada")
	}
}

func TestProspFiltrosValidar(t *testing.T) {
	ok := []ProspFiltros{
		{UFs: []string{"ce"}},
		{CNAEPrefixos: []string{"56"}, Nome: "padaria"},
		{CNPJs: []string{"11.222.333/0001-81", "11222333000181", "12.ABC.345/01DE-35"}, MEI: "sim"}, // com cnpjs o resto é ignorado
		{UFs: []string{"SP"}, MEI: "sim", IncluirPF: true},
	}
	for i, f := range ok {
		f.Normalizar()
		if err := f.Validar(); err != nil {
			t.Errorf("filtro %d deveria ser válido: %v", i, err)
		}
	}
	f := ProspFiltros{CNPJs: []string{"11.222.333/0001-81", "11222333000181", " 12.abc.345/01de-35"}, UFs: []string{" ce "}}
	f.Normalizar()
	if len(f.CNPJs) != 2 || f.CNPJs[1] != "12ABC34501DE35" || f.UFs[0] != "CE" {
		t.Fatalf("Normalizar deveria deduplicar CNPJs e pôr a UF em maiúsculas: %+v", f)
	}

	ruins := map[string]ProspFiltros{
		"sem obrigatório":       {Portes: []string{"me"}},
		"mei sem incluir_pf":    {UFs: []string{"CE"}, MEI: "sim"},
		"nome curto":            {CNAEs: []int{4721102}, Nome: "ab"},
		"nome sem cidade/cnae":  {UFs: []string{"CE"}, Nome: "padaria"},
		"bairro sem cidade":     {UFs: []string{"CE"}, Bairros: []string{"CENTRO"}},
		"prefixo de cnae ruim":  {CNAEPrefixos: []string{"5"}},
		"cnpj inválido":         {CNPJs: []string{"11222333000182"}},
		"uf com três letras":    {UFs: []string{"CEA"}},
		"prefixo com letras":    {CNAEPrefixos: []string{"5A"}},
		"prefixo longo de cnae": {CNAEPrefixos: []string{"562010"}},
	}
	for nome, f := range ruins {
		if err := f.Validar(); err == nil {
			t.Errorf("%s: esperava erro", nome)
		}
	}
	if !ProspOrdemValida("") || !ProspOrdemValida(OrdemAleatoria) || ProspOrdemValida(OrdemBase) || ProspOrdemValida("x") {
		t.Error("ProspOrdemValida aceita vazio e as ordens pedíveis; 'base' é só de resposta")
	}
}

func TestSelecaoEBloqueioSemBanco(t *testing.T) {
	sim := true
	validas := []ProspSelecao{
		{IDs: []int64{1, 2}},
		{Todos: true},
		{Todos: true, Filtro: &ProspItemFiltro{Estado: ItemNovo, ComCelular: &sim}, Exceto: []int64{9}},
	}
	for i, s := range validas {
		if err := s.Validar(); err != nil {
			t.Errorf("seleção %d deveria ser válida: %v", i, err)
		}
	}
	invalidas := []ProspSelecao{
		{},
		{IDs: []int64{1}, Todos: true},
		{IDs: []int64{1}, Exceto: []int64{2}},
		{IDs: make([]int64, MaxSelecaoIDs+1)},
		{Todos: true, Exceto: make([]int64, MaxSelecaoIDs+1)},
		{Todos: true, Filtro: &ProspItemFiltro{Estado: "inexistente"}},
	}
	for i, s := range invalidas {
		if err := s.Validar(); err == nil {
			t.Errorf("seleção inválida %d passou", i)
		}
	}

	normaliza := []struct{ tipo, entrada, quer string }{
		{BloqueioTelefone, "+55 (85) 99999-1234", "85999991234"},
		{BloqueioTelefone, "085 3222-1234", "8532221234"},
		{BloqueioEmail, "  Ana@Exemplo.COM.br ", "ana@exemplo.com.br"},
		{BloqueioCNPJ, "12.abc.345/01de-35", "12ABC34501DE35"},
	}
	for _, c := range normaliza {
		got, err := NormalizarValorBloqueio(c.tipo, c.entrada)
		if err != nil || got != c.quer {
			t.Errorf("NormalizarValorBloqueio(%s, %q) = %q, %v; quer %q", c.tipo, c.entrada, got, err, c.quer)
		}
	}
	for _, c := range [][2]string{{BloqueioTelefone, "99999-1234"}, {BloqueioEmail, "sem-arroba"}, {BloqueioCNPJ, "11222333000182"}, {"outro", "x"}} {
		if _, err := NormalizarValorBloqueio(c[0], c[1]); err == nil {
			t.Errorf("NormalizarValorBloqueio(%s, %q) deveria falhar", c[0], c[1])
		}
	}

	if MascararCPF("123.456.789-09") != "***456789**" || MascararCPF("") != "" || MascararCPF("1234") != "****" {
		t.Error("MascararCPF fora do formato ***123456**")
	}
	for _, r := range ResultadosLead {
		if !ResultadoValido(r) || OutcomeDoResultado(r) == "" || !ValidCallOutcome(OutcomeDoResultado(r)) {
			t.Errorf("resultado %s sem outcome válido em calls", r)
		}
	}
	if !ResultadoContaTentativa(ResultadoOcupado) || ResultadoContaTentativa(ResultadoNumeroErrado) ||
		!ResultadoConectado(ResultadoNaoLigarMais) || ResultadoConectado(ResultadoCaixaPostal) {
		t.Error("classificação de resultados fora do contrato")
	}
	if id := NovoPedidoID(); len(id) != 36 || id[14] != '4' || id == NovoPedidoID() {
		t.Errorf("NovoPedidoID deveria ser um UUID v4 único: %q", id)
	}
}

func TestPermissoesDeProspeccao(t *testing.T) {
	m := DefaultPermissions()
	quer := map[string][2]bool{ // seller, manager
		PermProspectingView:   {false, true},
		PermProspectingManage: {false, true},
		PermLeadsView:         {true, true},
		PermLeadsEdit:         {true, true},
		PermLeadsAssign:       {false, true},
	}
	for perm, q := range quer {
		if !ValidPermission(perm) {
			t.Errorf("%s deveria estar no catálogo", perm)
		}
		if m[RoleSeller][perm] != q[0] || m[RoleManager][perm] != q[1] || !m[RoleAdmin][perm] {
			t.Errorf("%s: seller=%v manager=%v admin=%v", perm, m[RoleSeller][perm], m[RoleManager][perm], m[RoleAdmin][perm])
		}
	}
	grupo := 0
	for _, p := range PermissionCatalog {
		if p.Group == "Prospecção" {
			grupo++
		}
	}
	if grupo != 5 {
		t.Errorf("o grupo Prospecção deveria ter 5 permissões, tem %d", grupo)
	}
}
