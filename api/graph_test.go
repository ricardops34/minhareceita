package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestGraphHandler(t *testing.T) {
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
			"/qsa/19131243000197",
			http.MethodGet,
			http.StatusOK,
			`{"company_id":"19131243000197","name":"OPEN KNOWLEDGE BRASIL","partners":[{"partner_id":"7f34c2ed2c1e8587d5598686e0c65360","name":"HAYDEE SVAB","cpf":"***112108**"}]}`,
		},
		{
			"/qsa/19.131.243/0001-97",
			http.MethodGet,
			http.StatusOK,
			`{"company_id":"19131243000197","name":"OPEN KNOWLEDGE BRASIL","partners":[{"partner_id":"7f34c2ed2c1e8587d5598686e0c65360","name":"HAYDEE SVAB","cpf":"***112108**"}]}`,
		},
		{
			"/cnpjs/7f34c2ed2c1e8587d5598686e0c65360",
			http.MethodGet,
			http.StatusOK,
			`{"partner_id":"7f34c2ed2c1e8587d5598686e0c65360","name":"HAYDEE SVAB","cpf":"***112108**","companies":[{"cnpj":"19131243000197","name":"OPEN KNOWLEDGE BRASIL"}]}`,
		},
		{
			"/cnpjs/999",
			http.MethodGet,
			http.StatusNotFound,
			`{"message":"Identificador 999 não encontrado."}`,
		},
		{
			"/qsa/00000000000000",
			http.MethodGet,
			http.StatusNotFound,
			`{"message":"CNPJ 00.000.000/0000-00 não encontrado."}`,
		},
		{
			"/qsa/invalid-id",
			http.MethodGet,
			http.StatusBadRequest,
			`{"message":"CNPJ invalid-id inválido."}`,
		},
		{
			"/other",
			http.MethodGet,
			http.StatusNotFound,
			`{"message":"Endpoint /other não encontrado. Use /qsa/<CNPJ> ou /cnpjs/<ID>."}`,
		},
		{
			"/qsa/19131243000197",
			http.MethodOptions,
			http.StatusOK,
			``,
		},
	}

	app := graphAPI{db: &mockGraphDatabase{}}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			req, err := http.NewRequest(c.method, c.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp := httptest.NewRecorder()
			app.handler(resp, req)

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
