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

func TestListFilesByName(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/2026-01/Cnaes.zip" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", "22078")
		w.Header().Set("Last-Modified", "Sun, 10 May 2026 19:23:10 GMT")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := &webdav{base: srv.URL + "/", token: "test-token", client: srv.Client()}
	entries, err := c.listFilesByName("2026-01", []string{"Cnaes.zip", "missing.zip"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].DisplayName != "Cnaes.zip" || entries[0].ContentLength != 22078 {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}

func TestListWithFallback(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			if r.URL.Path != "/2026-01/Cnaes.zip" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", "22078")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	c := &webdav{base: srv.URL + "/", token: "test-token", client: srv.Client()}
	entries, err := listWithFallback(c, "2026-01", []string{"Cnaes.zip", "missing.zip"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(entries) != 1 || entries[0].DisplayName != "Cnaes.zip" {
		t.Fatalf("unexpected entries: %+v", entries)
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
