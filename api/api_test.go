package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeberg.org/cuducos/minha-receita/db"
	"tangled.org/cuducos.me/go-cnpj"
)

type mockDatabase struct{ calls int }

func (d *mockDatabase) GetCompany(ctx context.Context, n string) ([]byte, error) {
	d.calls++
	n = cnpj.Unmask(n)
	if n != "19131243000197" {
		return nil, fmt.Errorf("cnpj %s: %w", n, db.ErrCompanyNotFound)
	}

	b, err := os.ReadFile(filepath.Join("..", "testdata", "response.json"))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (mockDatabase) Search(ctx context.Context, q *db.Query) ([]byte, error) { return nil, nil }

func (mockDatabase) MetaRead(k string) (string, error) { return "42", nil }

func (mockDatabase) AllCompanies(ctx context.Context, cursor *string, limit uint32) ([]string, *string, error) {
	return nil, nil, nil
}

func TestCompanyHandler(t *testing.T) {
	t.Parallel()
	f, err := filepath.Abs(filepath.Join("..", "testdata", "response.json"))
	if err != nil {
		t.Errorf("could understand path %s", f)
	}
	b, err := os.ReadFile(f)
	if err != nil {
		t.Errorf("could not read from %s", f)
	}
	expected := strings.TrimSpace(string(b))

	cases := []struct {
		method  string
		path    string
		status  int
		content string
	}{
		{
			http.MethodHead,
			"/",
			http.StatusMethodNotAllowed,
			`{"message":"Essa URL aceita apenas o método GET."}`,
		},
		{
			http.MethodOptions,
			"/",
			http.StatusOK,
			"",
		},
		{
			http.MethodHead,
			"/",
			http.StatusMethodNotAllowed,
			`{"message":"Essa URL aceita apenas o método GET."}`,
		},
		{
			http.MethodPost,
			"/",
			http.StatusMethodNotAllowed,
			`{"message":"Essa URL aceita apenas o método GET."}`,
		},
		{
			http.MethodGet,
			"/",
			http.StatusFound,
			"",
		},
		{
			http.MethodGet,
			"/foobar",
			http.StatusBadRequest,
			`{"message":"CNPJ foobar inválido."}`,
		},
		{
			http.MethodGet,
			"/12%2234",
			http.StatusBadRequest,
			`{"message":"CNPJ 1234 inválido."}`,
		},
		{
			http.MethodGet,
			"/%22%22%22",
			http.StatusBadRequest,
			`{"message":"CNPJ informado inválido."}`,
		},
		{
			http.MethodGet,
			"/00.000.000/0001-91",
			http.StatusNotFound,
			`{"message":"CNPJ 00.000.000/0001-91 não encontrado."}`,
		},
		{
			http.MethodGet,
			"/00000000000191",
			http.StatusNotFound,
			`{"message":"CNPJ 00.000.000/0001-91 não encontrado."}`,
		},
		{
			http.MethodGet,
			"/19.131.243/0001-97",
			http.StatusOK,
			expected,
		},
		{
			http.MethodGet,
			"/19131243000197",
			http.StatusOK,
			expected,
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%s %s", c.method, c.path), func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequest(c.method, c.path, nil)
			if err != nil {
				t.Fatal("expected an HTTP request, but got an error.")
			}
			if c.method == http.MethodPost {
				req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			}

			app := api{db: &mockDatabase{}}
			r := httptest.NewRecorder()
			h := http.HandlerFunc(app.companyHandler)
			h.ServeHTTP(r, req)

			if r.Code != c.status {
				t.Errorf("expected %s to return %v, but got %v", c.method, c.status, r.Code)
			}
			if c.content != "" {
				if b := strings.TrimSpace(r.Body.String()); b != c.content {
					t.Errorf("\nexpected HTTP contents to be:\n\t%s\ngot:\n\t%s", c.content, r.Body.String())
				}
				if c := r.Header().Get("Content-type"); c != "application/json" {
					t.Errorf("\nexpected content-type to be application/json, but got %s", c)
				}
			}
		})
	}
}

func TestInvalidCNPJResponseIsValidJSON(t *testing.T) {
	t.Parallel()
	for _, p := range []string{
		`/aspas%20%22%20no%20meio`,
		`/barra%20%5C%20invertida`,
		`/quebra%0Ade%20linha`,
		`/CNPJ%20ab%22cd%20inv%C3%A1lido.`,
	} {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequest(http.MethodGet, p, nil)
			if err != nil {
				t.Fatal("expected an HTTP request, but got an error.")
			}
			app := api{db: &mockDatabase{}}
			r := httptest.NewRecorder()
			http.HandlerFunc(app.companyHandler).ServeHTTP(r, req)

			if r.Code != http.StatusBadRequest {
				t.Fatalf("expected bad request for %s, but got %v", p, r.Code)
			}
			var got struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil {
				t.Fatalf("expected valid JSON for path %s, but got error %v (body: %s)", p, err, r.Body.String())
			}
		})
	}
}

type transientErrorDatabase struct{ mockDatabase }

func (d *transientErrorDatabase) GetCompany(ctx context.Context, n string) ([]byte, error) {
	d.calls++
	return nil, errors.New("connection refused")
}

func TestSingleCompanyTransientError(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequest(http.MethodGet, "/19.131.243/0001-97", nil)
	if err != nil {
		t.Fatal("expected an HTTP request, but got an error.")
	}

	app := api{db: &transientErrorDatabase{}}
	resp := httptest.NewRecorder()
	h := http.HandlerFunc(app.companyHandler)
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Errorf("expected a transient error to return %v, but got %v", http.StatusServiceUnavailable, resp.Code)
	}
	if cc := resp.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected Cache-Control to be no-store, but got %q", cc)
	}
}

func TestSingleCompanyNotFoundIsCached(t *testing.T) {
	t.Parallel()
	c, err := newCache(minCacheSize)
	if err != nil {
		t.Fatalf("could not create cache: %v", err)
	}
	d := &mockDatabase{}
	app := api{db: d, cache: c}

	do := func() *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, "/00.000.000/0001-91", nil)
		if err != nil {
			t.Fatal("expected an HTTP request, but got an error.")
		}
		resp := httptest.NewRecorder()
		http.HandlerFunc(app.companyHandler).ServeHTTP(resp, req)
		return resp
	}

	r := do()
	if r.Code != http.StatusNotFound {
		t.Errorf("expected first request to return %v, but got %v", http.StatusNotFound, r.Code)
	}
	c.r.Wait()

	r = do()
	if r.Code != http.StatusNotFound {
		t.Errorf("expected cached request to return %v, but got %v", http.StatusNotFound, r.Code)
	}
	if body := strings.TrimSpace(r.Body.String()); body != `{"message":"CNPJ 00.000.000/0001-91 não encontrado."}` {
		t.Errorf("unexpected cached 404 body: %s", body)
	}
	if d.calls != 1 {
		t.Errorf("expected the database to be queried once (second request served from cache), but it was queried %d times", d.calls)
	}
}

func TestSingleCompanyTransientErrorIsNotCached(t *testing.T) {
	t.Parallel()
	c, err := newCache(minCacheSize)
	if err != nil {
		t.Fatalf("could not create cache: %v", err)
	}
	d := &transientErrorDatabase{}
	app := api{db: d, cache: c}

	do := func() *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, "/19.131.243/0001-97", nil)
		if err != nil {
			t.Fatal("expected an HTTP request, but got an error.")
		}
		resp := httptest.NewRecorder()
		http.HandlerFunc(app.companyHandler).ServeHTTP(resp, req)
		return resp
	}

	r := do()
	if r.Code != http.StatusServiceUnavailable {
		t.Errorf("expected first request to return %v, but got %v", http.StatusServiceUnavailable, r.Code)
	}
	c.r.Wait()

	r = do()
	if r.Code != http.StatusServiceUnavailable {
		t.Errorf("expected second request to still return %v (not cached), but got %v", http.StatusServiceUnavailable, r.Code)
	}
	if d.calls != 2 {
		t.Errorf("expected the database to be queried on every request (transient error not cached), but it was queried %d times", d.calls)
	}
}

func TestHealthHandler(t *testing.T) {
	cases := []struct {
		method  string
		status  int
		content string
	}{
		{
			http.MethodGet,
			http.StatusOK,
			"",
		},
		{
			http.MethodPost,
			http.StatusMethodNotAllowed,
			`{"message":"Essa URL aceita apenas os métodos GET e HEAD."}`,
		},
		{
			http.MethodHead,
			http.StatusOK,
			"",
		},
	}

	for _, c := range cases {
		req, err := http.NewRequest(c.method, "/healthz", nil)
		if err != nil {
			t.Fatal("expected an HTTP request, but got an error.")
		}
		app := api{db: &mockDatabase{}}
		resp := httptest.NewRecorder()
		h := http.HandlerFunc(app.healthHandler)
		h.ServeHTTP(resp, req)

		if resp.Code != c.status {
			t.Errorf("expected %s /healthz to return %v, but got %v", c.method, c.status, resp.Code)
		}
		if strings.TrimSpace(resp.Body.String()) != c.content {
			t.Errorf("\nexpected HTTP contents to be %s, got %s", c.content, resp.Body.String())
		}
		if c.content != "" {
			if ct := resp.Result().Header.Get("Content-type"); ct != "application/json" {
				t.Errorf("expected content-type application/json, but got %s", ct)
			}
		}
	}
}

func TestUpdatedHandler(t *testing.T) {
	app := api{db: &mockDatabase{}}
	for _, c := range []struct {
		method  string
		status  int
		content string
	}{
		{http.MethodGet, http.StatusOK, `{"message":"42"}`},
		{http.MethodPost, http.StatusMethodNotAllowed, `{"message":"Essa URL aceita apenas o método GET."}`},
		{http.MethodHead, http.StatusMethodNotAllowed, `{"message":"Essa URL aceita apenas o método GET."}`},
		{http.MethodOptions, http.StatusMethodNotAllowed, `{"message":"Essa URL aceita apenas o método GET."}`},
	} {
		req, err := http.NewRequest(c.method, "/updated", nil)
		if err != nil {
			t.Fatal("expected an HTTP request, but got an error.")
		}
		resp := httptest.NewRecorder()
		h := http.HandlerFunc(app.updatedHandler)
		h.ServeHTTP(resp, req)

		if resp.Code != c.status {
			t.Errorf("expected %s /urls to return %v, but got %v", c.method, c.status, resp.Code)
		}
		if strings.TrimSpace(resp.Body.String()) != c.content {
			t.Errorf("\nexpected HTTP contents to be %s, got %s", c.content, resp.Body.String())
		}
		if c.content != "" {
			if ct := resp.Result().Header.Get("Content-type"); ct != "application/json" {
				t.Errorf("expected content-type application/json, but got %s", ct)
			}
		}
	}
}

func TestAllowedHostWrap(t *testing.T) {
	for _, c := range []struct {
		allowedHost string
		status      int
	}{
		{"", http.StatusOK},
		{"127.0.0.1", http.StatusOK},
		{"forty-two", http.StatusTeapot},
	} {
		t.Run(fmt.Sprintf("test returns %d when allowed host is %s", c.status, c.allowedHost), func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/19131243000197", nil)
			req.Host = "127.0.0.1"
			if err != nil {
				t.Fatal("expected an HTTP request, but got an error.")
			}
			resp := httptest.NewRecorder()
			app := api{db: &mockDatabase{}, host: c.allowedHost}
			h := http.HandlerFunc(app.allowedHostWrapper(app.companyHandler))
			h.ServeHTTP(resp, req)
			if resp.Code != c.status {
				t.Errorf("expected request with allowed host `%s` to return %d, but got %d (request header had `%s`) ", c.allowedHost, c.status, resp.Code, req.Host)
			}
		})
	}

}
