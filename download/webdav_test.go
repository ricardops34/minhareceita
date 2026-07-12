package download

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func server(t *testing.T, propfindFixture, getFile string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := r.BasicAuth()
		if !ok || user != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case "PROPFIND":
			if r.Header.Get("Depth") != "1" {
				t.Errorf("expected Depth: 1, got %q", r.Header.Get("Depth"))
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			if _, err := w.Write([]byte(propfindFixture)); err != nil {
				t.Errorf("could not write response: %v", err)
			}
		case "GET":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(getFile)); err != nil {
				t.Errorf("could not write response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

func TestList(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "propfind_2026-01.xml"))
	if err != nil {
		t.Fatal(err)
	}
	srv := server(t, string(b), "")
	defer srv.Close()
	c := &webdav{base: srv.URL + "/", token: "test-token", client: srv.Client()}
	entries, err := c.list("2026-01")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].DisplayName != "2026-01" || entries[0].Collection == nil {
		t.Errorf("expected first entry to be the directory, got %+v", entries[0])
	}
	if entries[1].DisplayName != "Cnaes.zip" || entries[1].Collection != nil {
		t.Errorf("expected second entry to be Cnaes.zip, got %+v", entries[1])
	}
	if entries[1].ContentLength != 22078 {
		t.Errorf("expected content length 22078, got %d", entries[1].ContentLength)
	}
}

func TestDownload(t *testing.T) {
	t.Parallel()
	srv := server(t, "", "test-content")
	defer srv.Close()
	c := &webdav{base: srv.URL + "/", token: "test-token", client: srv.Client()}
	var sb strings.Builder
	n, err := c.download("file.zip", &sb)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if n != int64(len("test-content")) {
		t.Errorf("expected %d bytes, got %d", len("test-content"), n)
	}
	if sb.String() != "test-content" {
		t.Errorf("expected test-content, got %q", sb.String())
	}
}

func TestListError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := &webdav{base: srv.URL + "/", token: "test-token", client: srv.Client()}
	if _, err := c.list("missing"); err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestDownloadError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := &webdav{base: srv.URL + "/", token: "test-token", client: srv.Client()}
	var sb strings.Builder
	if _, err := c.download("file.zip", &sb); err == nil {
		t.Error("expected an error, got nil")
	}
}
