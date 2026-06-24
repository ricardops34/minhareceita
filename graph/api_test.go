package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/cuducos/minha-receita/db"
)

func TestAPI(t *testing.T) {
	tmp, err := os.MkdirTemp("", "api_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			t.Logf("failed to remove temp dir %s: %v", tmp, err)
		}
	}()

	path := filepath.Join(tmp, "graph")

	data := []db.Relationship{
		{
			CompanyID:   "11111111000111",
			CompanyName: "Company A",
			PartnerID:   "22222222222",
			PartnerName: "Partner B",
			PartnerCPF:  "22222222222",
			PartnerType: 2,
		},
		{
			CompanyID:   "33333333000133",
			CompanyName: "Company C",
			PartnerID:   "22222222222",
			PartnerName: "Partner B",
			PartnerCPF:  "22222222222",
			PartnerType: 2,
		},
		{
			CompanyID:   "33333333000133",
			CompanyName: "Company C",
			PartnerID:   "44444444444",
			PartnerName: "Partner D",
			PartnerCPF:  "44444444444",
			PartnerType: 2,
		},
	}

	s := &mockStreamer{relationships: data}

	err = Create(context.Background(), s, int64(len(data)), path, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	srv, err := NewServer(path, 0)
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}
	defer srv.Close()

	t.Run("Relations Handler", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/relacoes/22222222222", nil)
		w := httptest.NewRecorder()
		srv.RelationsHandler(w, req)

		res := w.Result()
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Logf("failed to close response body: %v", err)
			}
		}()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}

		var rels []db.Relationship
		err := json.NewDecoder(res.Body).Decode(&rels)
		if err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(rels) != 2 {
			t.Errorf("expected 2 relationships, got %d", len(rels))
		}
	})

	t.Run("Connection Handler", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/conexao/11111111000111/33333333000133", nil)
		w := httptest.NewRecorder()
		srv.ConnectionHandler(w, req)

		res := w.Result()
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Logf("failed to close response body: %v", err)
			}
		}()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}

		var rels []db.Relationship
		err := json.NewDecoder(res.Body).Decode(&rels)
		if err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(rels) != 2 {
			t.Errorf("expected path of length 2, got %d", len(rels))
		}

		if rels[0].CompanyID != "11111111000111" || rels[0].PartnerID != "22222222222" {
			t.Errorf("unexpected first relationship: %+v", rels[0])
		}
		if rels[1].CompanyID != "33333333000133" || rels[1].PartnerID != "22222222222" {
			t.Errorf("unexpected second relationship: %+v", rels[1])
		}
	})
}
