package enriquecimento

import (
	"fmt"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
)

// AtribuirCampo grava o valor do perfil no struct da empresa (em memória),
// com a mesma lista branca de ApplyEnrichedFields. Devolve false para campo
// desconhecido ou valor incompatível.
func AtribuirCampo(c *models.Company, campo string, v any) bool {
	if c == nil || Vazio(v) {
		return false
	}
	texto := func() string { return strings.TrimSpace(fmt.Sprint(v)) }
	switch campo {
	case "name":
		c.Name = texto()
	case "domain":
		c.Domain = strings.ToLower(texto())
	case "phone":
		c.Phone = texto()
	case "industry":
		c.Industry = texto()
	case "city":
		c.City = texto()
	case "state":
		c.State = texto()
	case "legal_name":
		c.LegalName = texto()
	case "trade_name":
		c.TradeName = texto()
	case "cnae_code":
		c.CNAECode = texto()
	case "cnae_description":
		c.CNAEDescription = texto()
	case "legal_nature":
		c.LegalNature = texto()
	case "founded_at":
		d, err := models.ParseDate(texto())
		if err != nil || d == nil {
			return false
		}
		c.FoundedAt = d
	case "capital":
		f, ok := numero(v)
		if !ok {
			return false
		}
		c.Capital = &f
	case "company_size":
		c.CompanySize = texto()
	case "registration_status":
		c.RegistrationStatus = texto()
	case "registration_status_at":
		d, err := models.ParseDate(texto())
		if err != nil || d == nil {
			return false
		}
		c.RegistrationStatusAt = d
	case "simples":
		b, ok := v.(bool)
		if !ok {
			return false
		}
		c.Simples = &b
	case "mei":
		b, ok := v.(bool)
		if !ok {
			return false
		}
		c.MEI = &b
	case "street":
		c.Street = texto()
	case "street_number":
		c.StreetNumber = texto()
	case "complement":
		c.Complement = texto()
	case "neighborhood":
		c.Neighborhood = texto()
	case "zip":
		c.Zip = SomenteDigitos(texto())
	case "email":
		c.Email = strings.ToLower(texto())
	case "website":
		c.Website = texto()
	case "employees_range":
		c.EmployeesRange = texto()
	case "revenue_range":
		c.RevenueRange = texto()
	default:
		return false
	}
	return true
}
