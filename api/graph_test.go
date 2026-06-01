package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"codeberg.org/cuducos/minha-receita/db"
)

type mockGraphDatabase struct{}

func (mockGraphDatabase) GetCompanyPartners(_ context.Context, id string) (string, error) {
	if id == "19131243000197" {
		return `{"company_id":"19131243000197","name":"OPEN KNOWLEDGE BRASIL","partners":[{"partner_id":"7f34c2ed2c1e8587d5598686e0c65360","name":"HAYDEE SVAB","cpf":"***112108**"}]}`, nil
	}
	return "", fmt.Errorf("not found")
}

func (mockGraphDatabase) GetPartnerCompanies(_ context.Context, id string) (string, error) {
	if id == "7f34c2ed2c1e8587d5598686e0c65360" {
		return `{"partner_id":"7f34c2ed2c1e8587d5598686e0c65360","name":"HAYDEE SVAB","cpf":"***112108**","companies":[{"cnpj":"19131243000197","name":"OPEN KNOWLEDGE BRASIL"}]}`, nil
	}
	return "", fmt.Errorf("not found")
}

func (mockGraphDatabase) GetRelated(_ context.Context, id string) ([]db.GraphEdge, error) {
	ok := "19131243000197"
	pf := "7f34c2ed2c1e8587d5598686e0c65360"
	pj := "33683111000280"

	e1 := db.GraphEdge{CompanyID: ok, CompanyName: "OPEN KNOWLEDGE BRASIL", PartnerID: pf, PartnerName: "HAYDEE SVAB", PartnerCPF: "***112108**", PartnerType: 2}
	e2 := db.GraphEdge{CompanyID: pj, CompanyName: "PETRÓLEO BRASILEIRO S.A. - PETROBRAS", PartnerID: pf, PartnerName: "HAYDEE SVAB", PartnerCPF: "***112108**", PartnerType: 2}

	if id == ok {
		return []db.GraphEdge{e1}, nil
	}
	if id == pj {
		return []db.GraphEdge{e2}, nil
	}
	if id == pf {
		return []db.GraphEdge{e1, e2}, nil
	}
	return nil, nil
}

func TestGraphHandler(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path    string
		method  string
		status  int
		content string
	}{
		{
			"/",
			http.MethodGet,
			http.StatusFound,
			"",
		},
		{
			"/relacoes/19131243000197",
			http.MethodGet,
			http.StatusOK,
			`[{"cnpj":"19131243000197","razao_social":"OPEN KNOWLEDGE BRASIL","id":"7f34c2ed2c1e8587d5598686e0c65360","nome":"HAYDEE SVAB","cpf":"***112108**"}]`,
		},
		{
			"/relacoes/19.131.243/0001-97",
			http.MethodGet,
			http.StatusOK,
			`[{"cnpj":"19131243000197","razao_social":"OPEN KNOWLEDGE BRASIL","id":"7f34c2ed2c1e8587d5598686e0c65360","nome":"HAYDEE SVAB","cpf":"***112108**"}]`,
		},
		{
			"/relacoes/7f34c2ed2c1e8587d5598686e0c65360",
			http.MethodGet,
			http.StatusOK,
			`[{"cnpj":"19131243000197","razao_social":"OPEN KNOWLEDGE BRASIL","id":"7f34c2ed2c1e8587d5598686e0c65360","nome":"HAYDEE SVAB","cpf":"***112108**"},{"cnpj":"33683111000280","razao_social":"PETRÓLEO BRASILEIRO S.A. - PETROBRAS","id":"7f34c2ed2c1e8587d5598686e0c65360","nome":"HAYDEE SVAB","cpf":"***112108**"}]`,
		},
		{
			"/relacoes/999",
			http.MethodGet,
			http.StatusNotFound,
			`{"message":"Identificador 999 não encontrado ou sem conexões."}`,
		},
		{
			"/conexao/19131243000197/33683111000280",
			http.MethodGet,
			http.StatusOK,
			`[{"cnpj":"19131243000197","razao_social":"OPEN KNOWLEDGE BRASIL","id":"7f34c2ed2c1e8587d5598686e0c65360","nome":"HAYDEE SVAB","cpf":"***112108**"},{"cnpj":"33683111000280","razao_social":"PETRÓLEO BRASILEIRO S.A. - PETROBRAS","id":"7f34c2ed2c1e8587d5598686e0c65360","nome":"HAYDEE SVAB","cpf":"***112108**"}]`,
		},

		{
			"/conexao/19131243000197/00000000000000",
			http.MethodGet,
			http.StatusNotFound,
			`{"message":"Nenhuma conexão encontrada entre 19131243000197 e 00000000000000."}`,
		},
		{
			"/other",
			http.MethodGet,
			http.StatusNotFound,
			`{"message":"Endpoint /other não encontrado. Use /relacoes/<ID> ou /conexao/<ID>/<ID>."}`,
		},
		{
			"/relacoes/19131243000197",
			http.MethodOptions,
			http.StatusOK,
			``,
		},
	}

	app := graphAPI{db: &mockGraphDatabase{}}
	mux := app.mux()

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequest(c.method, c.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)

			if resp.Code != c.status {
				t.Errorf("expected status %d, got %d", c.status, resp.Code)
			}
			if c.content != "" {
				if got := resp.Body.String(); got != c.content {
					t.Errorf("expected content %s, got %s", c.content, got)
				}
			}
		})
	}
}
