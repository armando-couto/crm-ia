package models

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Chaves de app_settings da prospecção.
const (
	SettingProspeccao   = "prospeccao"
	SettingMotivosPerda = "motivos_perda"
)

// Teto do sistema para a janela de contato (telemarketing): 8h às 20h.
const (
	JanelaHoraMinima = 8
	JanelaHoraMaxima = 20
)

// Códigos reservados de leads.motivo_descarte (os demais valores são rótulos
// de ConfigProspeccao.MotivosDescarte).
const (
	DescarteSemTelefone = "sem_telefone" // sem telefone válido
	DescarteSemContato  = "sem_contato"  // sem contato após as tentativas
	DescarteOptOut      = "opt_out"      // pediu para não ligar
	DescarteManual      = "manual"
)

// MotivosDescarteReservados não podem ser usados como rótulo configurável.
var MotivosDescarteReservados = []string{DescarteSemTelefone, DescarteSemContato, DescarteOptOut, DescarteManual}

// ConfigProspeccao são as configurações de prospecção do ambiente
// (GET/PUT /settings/prospeccao, chave "prospeccao" de app_settings).
type ConfigProspeccao struct {
	JanelaInicio         int      `json:"janela_inicio"`
	JanelaFim            int      `json:"janela_fim"`
	Sabado               bool     `json:"sabado"`
	MaxTentativas        int      `json:"max_tentativas"`
	IntervalosHoras      []int    `json:"intervalos_horas"`
	TentativasDia        int      `json:"tentativas_dia"`
	RetornoPadraoHoras   int      `json:"retorno_padrao_horas"`
	MetaLigacoesDia      int      `json:"meta_ligacoes_dia"`
	ExigirOptinWhatsApp  bool     `json:"exigir_optin_whatsapp"`
	IncluirSemDonoNaFila bool     `json:"incluir_sem_dono_na_fila"`
	PipelineID           *int64   `json:"pipeline_id"`
	StageID              *int64   `json:"stage_id"`
	PlaybookID           *int64   `json:"playbook_id"`
	MotivosDescarte      []string `json:"motivos_descarte"`
}

// DefaultConfigProspeccao são os valores de um ambiente novo.
func DefaultConfigProspeccao() ConfigProspeccao {
	return ConfigProspeccao{
		JanelaInicio:        9,
		JanelaFim:           18,
		Sabado:              false,
		MaxTentativas:       6,
		IntervalosHoras:     []int{3, 24, 48, 72, 120},
		TentativasDia:       2,
		RetornoPadraoHoras:  48,
		MetaLigacoesDia:     40,
		ExigirOptinWhatsApp: true,
		MotivosDescarte:     []string{"Sem interesse", "Já tem fornecedor", "Sem orçamento", "Fora do perfil", "Empresa fechada"},
	}
}

// LoadConfigProspeccao lê as configurações sobre os padrões. O que estiver
// gravado fora das regras (edição manual do banco) cai no padrão.
func LoadConfigProspeccao(db *sql.DB) ConfigProspeccao {
	c := DefaultConfigProspeccao()
	_, _ = GetSetting(db, SettingProspeccao, &c)
	if c.Validate() != nil {
		padrao := DefaultConfigProspeccao()
		padrao.PipelineID, padrao.StageID, padrao.PlaybookID = c.PipelineID, c.StageID, c.PlaybookID
		return padrao
	}
	return c
}

// SaveConfigProspeccao valida e grava.
func SaveConfigProspeccao(db *sql.DB, c *ConfigProspeccao, userID *int64) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return SetSetting(db, SettingProspeccao, c, userID)
}

// Validate confere limites e normaliza os motivos (mensagem pronta para 422).
func (c *ConfigProspeccao) Validate() error {
	if c.JanelaInicio < JanelaHoraMinima || c.JanelaFim > JanelaHoraMaxima {
		return fmt.Errorf("a janela de contato precisa ficar entre %dh e %dh", JanelaHoraMinima, JanelaHoraMaxima)
	}
	if c.JanelaInicio >= c.JanelaFim {
		return fmt.Errorf("o início da janela de contato precisa ser antes do fim")
	}
	if c.MaxTentativas < 1 || c.MaxTentativas > 20 {
		return fmt.Errorf("o máximo de tentativas deve ficar entre 1 e 20")
	}
	if len(c.IntervalosHoras) > 20 {
		return fmt.Errorf("informe até 20 intervalos de cadência")
	}
	for _, h := range c.IntervalosHoras {
		if h < 1 || h > 720 {
			return fmt.Errorf("cada intervalo da cadência deve ficar entre 1 e 720 horas")
		}
	}
	if c.IntervalosHoras == nil {
		c.IntervalosHoras = []int{}
	}
	if c.TentativasDia < 1 || c.TentativasDia > 10 {
		return fmt.Errorf("as tentativas por dia devem ficar entre 1 e 10")
	}
	if c.RetornoPadraoHoras < 1 || c.RetornoPadraoHoras > 720 {
		return fmt.Errorf("o retorno padrão deve ficar entre 1 e 720 horas")
	}
	if c.MetaLigacoesDia < 0 || c.MetaLigacoesDia > 1000 {
		return fmt.Errorf("a meta diária de ligações deve ficar entre 0 e 1000")
	}
	if c.StageID != nil && c.PipelineID == nil {
		return fmt.Errorf("informe o funil da etapa de destino")
	}
	motivos, err := normalizarMotivos(c.MotivosDescarte, "descarte")
	if err != nil {
		return err
	}
	for _, m := range motivos {
		if slices.Contains(MotivosDescarteReservados, strings.ToLower(m)) {
			return fmt.Errorf("o motivo %q é reservado pelo sistema", m)
		}
	}
	c.MotivosDescarte = motivos
	return nil
}

// normalizarMotivos apara, tira vazios e repetidos (sem diferenciar caixa) e
// confere tamanho (coluna VARCHAR(80)) e quantidade.
func normalizarMotivos(motivos []string, qual string) ([]string, error) {
	out := make([]string, 0, len(motivos))
	for _, m := range motivos {
		m = strings.Join(strings.Fields(m), " ")
		if m == "" {
			continue
		}
		if len([]rune(m)) > 80 {
			return nil, fmt.Errorf("motivo de %s longo demais (até 80 caracteres): %q", qual, m)
		}
		if !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, m) }) {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("informe ao menos um motivo de %s", qual)
	}
	if len(out) > 30 {
		return nil, fmt.Errorf("informe até 30 motivos de %s", qual)
	}
	return out, nil
}

// MotivoDescarteValido diz se o texto é um dos rótulos configurados.
func (c ConfigProspeccao) MotivoDescarteValido(motivo string) bool {
	motivo = strings.TrimSpace(motivo)
	return motivo != "" && slices.Contains(c.MotivosDescarte, motivo)
}

// IntervaloCadencia devolve a espera depois da n-ésima tentativa sem contato
// (n começa em 1): intervalos_horas[min(n, len) - 1] em horas corridas; lista
// vazia = 24 h.
func (c ConfigProspeccao) IntervaloCadencia(tentativas int) time.Duration {
	if len(c.IntervalosHoras) == 0 {
		return 24 * time.Hour
	}
	i := min(max(tentativas, 1), len(c.IntervalosHoras)) - 1
	return time.Duration(c.IntervalosHoras[i]) * time.Hour
}

// -----------------------------------------------------------------------------
// Janela de contato: dias úteis entre janela_inicio e janela_fim no fuso do
// ambiente (time.Local); sábado só se ligado; domingos e feriados nacionais nunca.
// -----------------------------------------------------------------------------

// Janela é o estado da janela de contato num instante (interface Janela).
type Janela struct {
	Aberta   bool       `json:"aberta"`
	Texto    string     `json:"texto"`
	ReabreEm *time.Time `json:"reabre_em"`
}

// DiaPermitido diz se o dia de t (no fuso do ambiente) aceita ligações.
func (c ConfigProspeccao) DiaPermitido(t time.Time) bool {
	t = t.In(time.Local)
	switch t.Weekday() {
	case time.Sunday:
		return false
	case time.Saturday:
		if !c.Sabado {
			return false
		}
	}
	return !FeriadoNacional(t)
}

// JanelaAberta diz se t está dentro da janela de contato.
func (c ConfigProspeccao) JanelaAberta(t time.Time) bool {
	t = t.In(time.Local)
	return c.DiaPermitido(t) && t.Hour() >= c.JanelaInicio && t.Hour() < c.JanelaFim
}

// inicioDaJanela devolve janela_inicio no dia de t.
func (c ConfigProspeccao) inicioDaJanela(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), c.JanelaInicio, 0, 0, 0, time.Local)
}

// ProximoDiaPermitido devolve janela_inicio do próximo dia permitido DEPOIS do
// dia de t (é para onde vai a tentativa que estourou tentativas_dia).
func (c ConfigProspeccao) ProximoDiaPermitido(t time.Time) time.Time {
	d := c.inicioDaJanela(t)
	for i := 0; i < 30; i++ { // nunca há 30 dias seguidos sem expediente
		d = d.AddDate(0, 0, 1)
		if c.DiaPermitido(d) {
			break
		}
	}
	return d
}

// AjustarJanela desloca um instante para dentro da janela: dentro dela, fica
// como está; antes da abertura de um dia permitido, vai para a abertura desse
// dia; nos demais casos, para a abertura do próximo dia permitido.
func (c ConfigProspeccao) AjustarJanela(t time.Time) time.Time {
	t = t.In(time.Local)
	if c.JanelaAberta(t) {
		return t
	}
	if c.DiaPermitido(t) && t.Hour() < c.JanelaInicio {
		return c.inicioDaJanela(t)
	}
	return c.ProximoDiaPermitido(t)
}

// TextoJanela descreve a janela: "segunda a sexta, 9h às 18h".
func (c ConfigProspeccao) TextoJanela() string {
	dias := "segunda a sexta"
	if c.Sabado {
		dias = "segunda a sábado"
	}
	return fmt.Sprintf("%s, %dh às %dh", dias, c.JanelaInicio, c.JanelaFim)
}

// JanelaEm monta o estado da janela no instante informado.
func (c ConfigProspeccao) JanelaEm(agora time.Time) Janela {
	j := Janela{Aberta: c.JanelaAberta(agora), Texto: c.TextoJanela()}
	if !j.Aberta {
		reabre := c.AjustarJanela(agora)
		j.ReabreEm = &reabre
	}
	return j
}

// FeriadoNacional diz se a data (no fuso do ambiente) é feriado nacional:
// os de data fixa e a Sexta-feira Santa. Carnaval e Corpus Christi são pontos
// facultativos e não entram.
func FeriadoNacional(t time.Time) bool {
	t = t.In(time.Local)
	dia, mes := t.Day(), t.Month()
	switch {
	case mes == time.January && dia == 1, // Confraternização Universal
		mes == time.April && dia == 21,                        // Tiradentes
		mes == time.May && dia == 1,                           // Dia do Trabalho
		mes == time.September && dia == 7,                     // Independência
		mes == time.October && dia == 12,                      // Nossa Senhora Aparecida
		mes == time.November && dia == 2,                      // Finados
		mes == time.November && dia == 15,                     // Proclamação da República
		mes == time.November && dia == 20 && t.Year() >= 2024, // Consciência Negra (Lei 14.759/2023)
		mes == time.December && dia == 25:                     // Natal
		return true
	}
	sexta := pascoa(t.Year()).AddDate(0, 0, -2)
	return mes == sexta.Month() && dia == sexta.Day()
}

// pascoa calcula o domingo de Páscoa (algoritmo de Meeus/Jones/Butcher).
func pascoa(ano int) time.Time {
	a := ano % 19
	b, c := ano/100, ano%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	mes := (h + l - 7*m + 114) / 31
	dia := (h+l-7*m+114)%31 + 1
	return time.Date(ano, time.Month(mes), dia, 0, 0, 0, 0, time.Local)
}

// -----------------------------------------------------------------------------
// Motivos de perda de negócio (GET/PUT /settings/motivos-perda).
// -----------------------------------------------------------------------------

// MotivosPerda é a lista de motivos de perda e se informar um é obrigatório.
type MotivosPerda struct {
	Motivos []string `json:"motivos"`
	Exigir  bool     `json:"exigir"`
}

// MotivoPerdaVazio é o rótulo de negócios perdidos sem motivo em perdas_por_motivo.
const MotivoPerdaVazio = "Sem motivo informado"

// DefaultMotivosPerda são os motivos de um ambiente novo (exigir ligado).
func DefaultMotivosPerda() MotivosPerda {
	return MotivosPerda{
		Motivos: []string{"Preço", "Sem orçamento", "Escolheu concorrente", "Sem interesse", "Sem resposta", "Momento errado", "Outro"},
		Exigir:  true,
	}
}

// LoadMotivosPerda lê a configuração sobre os padrões.
func LoadMotivosPerda(db *sql.DB) MotivosPerda {
	m := DefaultMotivosPerda()
	_, _ = GetSetting(db, SettingMotivosPerda, &m)
	if m.Validate() != nil {
		return DefaultMotivosPerda()
	}
	return m
}

// SaveMotivosPerda valida e grava.
func SaveMotivosPerda(db *sql.DB, m *MotivosPerda, userID *int64) error {
	if err := m.Validate(); err != nil {
		return err
	}
	return SetSetting(db, SettingMotivosPerda, m, userID)
}

// Validate normaliza a lista (deals.lost_reason é VARCHAR(80)).
func (m *MotivosPerda) Validate() error {
	motivos, err := normalizarMotivos(m.Motivos, "perda")
	if err != nil {
		return err
	}
	m.Motivos = motivos
	return nil
}

// Contem diz se o motivo está na lista configurada.
func (m MotivosPerda) Contem(motivo string) bool {
	motivo = strings.TrimSpace(motivo)
	return motivo != "" && slices.Contains(m.Motivos, motivo)
}

// ValidarPerda aplica a regra do fechamento como perdido: com exigir, motivo
// vazio é erro ("informe o motivo da perda"); motivo informado precisa estar
// na lista. Devolve o motivo aparado.
func (m MotivosPerda) ValidarPerda(motivo string) (string, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		if m.Exigir {
			return "", ErrMotivoPerdaObrigatorio
		}
		return "", nil
	}
	if !m.Contem(motivo) {
		return "", fmt.Errorf("motivo de perda desconhecido: %s", motivo)
	}
	return motivo, nil
}

// ErrMotivoPerdaObrigatorio vira 422 com codigo "motivo_obrigatorio".
var ErrMotivoPerdaObrigatorio = fmt.Errorf("informe o motivo da perda")
