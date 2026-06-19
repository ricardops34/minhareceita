package company

import (
	"testing"
)

func TestMaskCPF(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		// MEI patterns (company name + CPF)
		{"João Silva 12345678901", "João Silva ***45678***"},
		{"Maria Santos ME 98765432109", "Maria Santos ME ***65432***"},
		{"JOSE DA SILVA 11122233344", "JOSE DA SILVA ***22233***"},
		{"COMERCIO DE ALIMENTOS LTDA 55566677788", "COMERCIO DE ALIMENTOS LTDA ***66677***"},
		// Edge cases with non-digit before CPF
		{"Empresa-12345678901", "Empresa-***45678***"},
		{"Nome 12345678901", "Nome ***45678***"},
		{"A12345678901", "A***45678***"},
		// Should NOT mask: 12 consecutive digits (not CPF pattern)
		{"Empresa123456789012", "Empresa123456789012"},
		{"000012345678901", "000012345678901"},
		// Should NOT mask: too short
		{"1234567890", "1234567890"},
		{"Short", "Short"},
		// Should NOT mask: non-digits in tail
		{"NomeEmpresa1234567890X", "NomeEmpresa1234567890X"},
		{"Empresa 1234567890a", "Empresa 1234567890a"},
		{"Test 123456-78901", "Test 123456-78901"},
		// Exactly 11 chars (all digits)
		{"12345678901", "***45678***"},
		// UTF-8 cases
		{"João José 12345678901", "João José ***45678***"},
		{"Quitanda São Miguel 99988877766", "Quitanda São Miguel ***88877***"},
		{"Café é Bom 12312312312", "Café é Bom ***12312***"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := maskCPF(tc.name)
			if got != tc.want {
				t.Errorf("expected masked %s to be %s, got %s", tc.name, tc.want, got)
			}
		})
	}
}
