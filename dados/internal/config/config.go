// Package config lê o ambiente do serviço de dados públicos.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Endereços conferidos em 21 e 22/11/2026. O token do compartilhamento da
// Receita pode mudar, por isso é configuração.
const (
	CNPJURLPadrao = "https://arquivos.receitafederal.gov.br/public.php/dav/files/YggdBLfdninEJX9/"
	PGFNURLPadrao = "https://dadosabertos.pgfn.gov.br/"
)

// Config é tudo que o serviço precisa para carregar e servir a base.
type Config struct {
	Ambiente    string
	Porta       string
	DatabaseURL string
	Token       string

	CNPJURL string
	PGFNURL string
	// DirTrabalho guarda os zips baixados até a troca de base.
	DirTrabalho string
	// CargaAutomatica confere as fontes na subida e a cada IntervaloCarga.
	CargaAutomatica bool
	IntervaloCarga  time.Duration
	// Amostra serve a base fictícia embutida e nunca acessa a rede.
	Amostra bool
	// UFs recorta a carga (vazio = Brasil).
	UFs []string
	// Conexoes é o número de downloads simultâneos (o firewall da Receita
	// rejeita rajadas).
	Conexoes int
	// EspacoEntreRequisicoes é o intervalo mínimo entre requisições novas à
	// mesma fonte.
	EspacoEntreRequisicoes time.Duration
	// DiscoBancoGB é o espaço livre que o operador declara existir no disco
	// do PostgreSQL (o serviço não enxerga o disco do host).
	DiscoBancoGB int
	// TempoBusca é o statement_timeout das consultas de busca.
	TempoBusca time.Duration
	// EsperaPublicacao é o intervalo mínimo entre as duas listagens iguais
	// que tornam uma referência do CNPJ elegível.
	EsperaPublicacao time.Duration
}

func (c Config) EmDesenvolvimento() bool { return c.Ambiente == "development" }

var reUF = regexp.MustCompile(`^[A-Z]{2}$`)

// Carregar lê o ambiente e valida o essencial.
func Carregar() (Config, error) {
	c := Config{
		Ambiente:               texto("APP_ENV", "production"),
		Porta:                  texto("PORT", "7896"),
		DatabaseURL:            texto("DATABASE_URL", ""),
		Token:                  texto("DADOS_TOKEN", ""),
		CNPJURL:                comBarra(texto("DADOS_CNPJ_URL", CNPJURLPadrao)),
		PGFNURL:                comBarra(texto("DADOS_PGFN_URL", PGFNURLPadrao)),
		DirTrabalho:            texto("DADOS_DIR_TRABALHO", "/trabalho"),
		CargaAutomatica:        booleano("DADOS_CARGA_AUTOMATICA", true),
		IntervaloCarga:         duracao("DADOS_CARGA_INTERVALO", 6*time.Hour),
		Amostra:                booleano("DADOS_AMOSTRA", false),
		Conexoes:               inteiro("DADOS_CONEXOES", 2),
		EspacoEntreRequisicoes: duracao("DADOS_ESPACO_REQUISICOES", 2500*time.Millisecond),
		DiscoBancoGB:           inteiro("DADOS_DISCO_MINIMO_GB", 70),
		TempoBusca:             duracao("DADOS_TEMPO_BUSCA", 4*time.Second),
		EsperaPublicacao:       duracao("DADOS_ESPERA_PUBLICACAO", 30*time.Minute),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL não definida")
	}
	if c.Token == "" {
		return c, errors.New("DADOS_TOKEN não definido")
	}
	if len(c.Token) < 24 {
		return c, errors.New("DADOS_TOKEN curto demais; gere com: openssl rand -hex 32")
	}
	if c.Conexoes < 1 || c.Conexoes > 4 {
		return c, errors.New("DADOS_CONEXOES deve ficar entre 1 e 4")
	}
	ufs, err := listaUFs(os.Getenv("DADOS_UFS"))
	if err != nil {
		return c, err
	}
	c.UFs = ufs
	return c, nil
}

func listaUFs(v string) ([]string, error) {
	vistos := map[string]bool{}
	saida := []string{}
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		uf := strings.ToUpper(strings.TrimSpace(p))
		if !reUF.MatchString(uf) {
			return nil, fmt.Errorf("DADOS_UFS inválida: %q não é uma UF", p)
		}
		if !vistos[uf] {
			vistos[uf] = true
			saida = append(saida, uf)
		}
	}
	sort.Strings(saida)
	return saida, nil
}

func comBarra(u string) string {
	if u == "" || strings.HasSuffix(u, "/") {
		return u
	}
	return u + "/"
}

func texto(nome, padrao string) string {
	if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
		return v
	}
	return padrao
}

func inteiro(nome string, padrao int) int {
	if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return padrao
}

func booleano(nome string, padrao bool) bool {
	if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return padrao
}

func duracao(nome string, padrao time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			return d
		}
	}
	return padrao
}
