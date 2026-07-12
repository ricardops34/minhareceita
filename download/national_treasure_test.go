package download

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCKANURLs(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "ckan_tabmun.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(b); err != nil {
			t.Errorf("could not write response: %v", err)
		}
	}))
	defer srv.Close()
	urls, err := ckanURLs(srv.URL, "test-pkg")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(urls) != 1 {
		t.Fatalf("expected 1 url, got %d", len(urls))
	}
	if urls[0] != "https://example.com/tabmun.csv" {
		t.Errorf("expected https://example.com/tabmun.csv, got %s", urls[0])
	}
}

func TestCKANURLsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := ckanURLs(srv.URL, "test-pkg"); err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestCKANURLsNotSuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"success":false,"result":{"resources":[]}}`)); err != nil {
			t.Errorf("could not write response: %v", err)
		}
	}))
	defer srv.Close()
	if _, err := ckanURLs(srv.URL, "test-pkg"); err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestSimpleDownload(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("test-content")); err != nil {
			t.Errorf("could not write response: %v", err)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	pth := filepath.Join(dir, "tabmun.csv")
	n, err := downloadURL(srv.URL+"/tabmun.csv", pth)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if n != int64(len("test-content")) {
		t.Errorf("expected %d bytes, got %d", len("test-content"), n)
	}
	b, err := os.ReadFile(pth)
	if err != nil {
		t.Fatalf("could not read file: %v", err)
	}
	if string(b) != "test-content" {
		t.Errorf("expected test-content, got %q", string(b))
	}
}
