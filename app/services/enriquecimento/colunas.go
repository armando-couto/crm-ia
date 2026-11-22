package enriquecimento

import "strings"

// TamanhoColuna é o tamanho (em caracteres) das colunas de texto de companies
// que o enriquecimento preenche (migrações 001, 027 e 028). Texto maior que a
// coluna faria o UPDATE inteiro falhar ("value too long"): é cortado antes.
var TamanhoColuna = map[string]int{
	"name": 200, "domain": 150, "phone": 30, "industry": 255, "city": 100, "state": 50,
	"legal_name": 255, "trade_name": 255, "cnae_code": 10, "cnae_description": 255,
	"legal_nature": 120, "company_size": 30, "registration_status": 30,
	"street": 255, "street_number": 20, "complement": 120, "neighborhood": 120, "zip": 10,
	"email": 255, "website": 255, "employees_range": 40, "revenue_range": 60,
}

// TruncarTexto corta s em n caracteres (runas, nunca no meio de um caractere
// acentuado) e apara os espaços.
func TruncarTexto(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n]))
}

// TruncarCampo corta o valor de texto ao tamanho da coluna do campo; valores
// que não são texto (data, número, booleano) e campos sem tamanho voltam como
// vieram.
func TruncarCampo(campo string, v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	if n, ok := TamanhoColuna[campo]; ok {
		return TruncarTexto(s, n)
	}
	return s
}
