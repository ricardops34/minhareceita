package download

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fileServer(t *testing.T, propfind string, files map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := r.BasicAuth()
		if !ok {
			t.Fatal("could not authenticate test file server")
		}
		if user != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			if _, err := io.WriteString(w, propfind); err != nil {
				t.Errorf("could not write response: %v", err)
			}
		case "GET":
			content, ok := files[strings.TrimPrefix(r.URL.Path, "/public.php/webdav/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if _, err := io.WriteString(w, content); err != nil {
				t.Errorf("could not write response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

func TestValidateYearMonth(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in string
		ok bool
	}{
		{"2026-06", true},
		{"202606", true},
		{"2026-1", false},
		{"2026-13", false},
		{"2026-00", false},
		{"7865-21", false},
		{"20260601", false},
		{"2026-06-01", false},
		{"invalid", false},
		{"", false},
	} {
		err := validateYearMonth(c.in)
		if !c.ok && err == nil {
			t.Errorf("expected error for %q, got nil", c.in)
		}
		if c.ok && err != nil {
			t.Errorf("expected no error for %q, got %v", c.in, err)
		}
	}
}

func TestDownloadFederalRevenue(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "propfind_2026-01.xml"))
	if err != nil {
		t.Fatal(err)
	}
	ls := map[string]string{
		"2026-01/Cnaes.zip":     "cnaes-data",
		"2026-01/Empresas0.zip": "empresas-data",
	}
	srv := fileServer(t, string(b), ls)
	defer srv.Close()
	c := &webdav{base: srv.URL + "/public.php/webdav/", token: "test-token", client: srv.Client()}
	dir := t.TempDir()
	if err := downloadFederalRevenue(c, "2026-01", dir); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	for name, want := range ls {
		b, err := os.ReadFile(filepath.Join(dir, filepath.Base(name)))
		if err != nil {
			t.Errorf("could not read %s: %v", name, err)
			continue
		}
		if string(b) != want {
			t.Errorf("expected %s, got %q", want, string(b))
		}
	}
}

func TestDownloadTaxRegime(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "propfind_tax_regime.xml"))
	if err != nil {
		t.Fatal(err)
	}
	ls := map[string]string{
		"entidades-imunes-e-isentas.zip": "imunes-data",
		"entidades-lucro-arbitrado.zip":  "arbitrado-data",
		"entidades-lucro-presumido.zip":  "presumido-data",
		"entidades-lucro-real.zip":       "lucro-real-data",
	}
	srv := fileServer(t, string(b), ls)
	defer srv.Close()
	c := &webdav{base: srv.URL + "/public.php/webdav/", token: "test-token", client: srv.Client()}
	dir := t.TempDir()
	if err := downloadTaxRegime(c, dir); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	b, err = os.ReadFile(filepath.Join(dir, "entidades-imunes-e-isentas.zip"))
	if err != nil {
		t.Fatalf("could not read file: %v", err)
	}
	if string(b) != "imunes-data" {
		t.Errorf("expected imunes-data, got %q", string(b))
	}
}

func TestDownloadEndToEnd(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "propfind_2026-01.xml"))
	if err != nil {
		t.Fatal(err)
	}
	cnpj := fileServer(t, string(b), map[string]string{
		"2026-01/Cnaes.zip":     "cnaes-data",
		"2026-01/Empresas0.zip": "empresas-data",
	})
	defer cnpj.Close()

	b, err = os.ReadFile(filepath.Join("..", "testdata", "propfind_tax_regime.xml"))
	if err != nil {
		t.Fatal(err)
	}
	tax := fileServer(t, string(b), map[string]string{
		"entidades-imunes-e-isentas.zip": "imunes-data",
		"entidades-lucro-arbitrado.zip":  "arbitrado-data",
		"entidades-lucro-presumido.zip":  "presumido-data",
		"entidades-lucro-real.zip":       "lucro-real-data",
	})
	defer tax.Close()

	b, err = os.ReadFile(filepath.Join("..", "testdata", "ckan_tabmun.json"))
	if err != nil {
		t.Fatal(err)
	}
	ckan := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(b); err != nil {
			t.Errorf("could not write response: %v", err)
		}
	}))
	defer ckan.Close()

	dir := t.TempDir()
	c1 := &webdav{base: cnpj.URL + "/public.php/webdav/", token: "test-token", client: cnpj.Client()}
	c2 := &webdav{base: tax.URL + "/public.php/webdav/", token: "test-token", client: tax.Client()}

	if err := downloadFederalRevenue(c1, "2026-01", dir); err != nil {
		t.Fatalf("expected no error downloading CNPJ, got %v", err)
	}
	if err := downloadTaxRegime(c2, dir); err != nil {
		t.Fatalf("expected no error downloading tax regime, got %v", err)
	}

	c, err := os.ReadFile(filepath.Join(dir, "Cnaes.zip"))
	if err != nil {
		t.Fatalf("could not read CNPJ file: %v", err)
	}
	if string(c) != "cnaes-data" {
		t.Errorf("expected cnaes-data, got %q", string(c))
	}

	got, err := os.ReadFile(filepath.Join(dir, "entidades-imunes-e-isentas.zip"))
	if err != nil {
		t.Fatalf("could not read tax regime file: %v", err)
	}
	if string(got) != "imunes-data" {
		t.Errorf("expected imunes-data, got %q", string(got))
	}
}
