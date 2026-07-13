package graph

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"codeberg.org/cuducos/minha-receita/company"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	data := []company.Relationship{
		{
			CompanyID:   "33683111000280",
			CompanyName: "Company A",
			PartnerID:   "3573de271293797f2abddc036be8f35e",
			PartnerName: "Partner B",
			PartnerCPF:  "***123456**",
			PartnerType: 2,
		},
		{
			CompanyID:   "34712359000103",
			CompanyName: "Company C",
			PartnerID:   "3573de271293797f2abddc036be8f35e",
			PartnerName: "Partner B",
			PartnerCPF:  "***123456**",
			PartnerType: 2,
		},
		{
			CompanyID:   "34712359000103",
			CompanyName: "Company C",
			PartnerID:   "70ec112375aec9de541ae0b7c54d7cac",
			PartnerName: "Partner D",
			PartnerCPF:  "***654321**",
			PartnerType: 2,
		},
	}

	w, err := NewWriter(path)
	if err != nil {
		t.Fatalf("writer failed: %v", err)
	}
	var close sync.Once
	defer close.Do(func() {
		if err := w.Close(); err != nil {
			t.Errorf("expected no error closing writer, got %q", err)
		}
	})

	for _, r := range data {
		if err := w.Save(&r); err != nil {
			t.Errorf("expected no error saving relationship, got %q", err)
		}
	}

	close.Do(func() {
		if err := w.Close(); err != nil {
			t.Fatalf("expected no error closing writer, got %q", err)
		}
	})

	srv, err := NewServer(path, 0)
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}
	defer srv.Close()

	t.Run("Relations Handler", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/3573de271293797f2abddc036be8f35e", nil)
		w := httptest.NewRecorder()
		srv.handler(w, req)

		res := w.Result()
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Logf("failed to close response body: %v", err)
			}
		}()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}

		var rels []company.Relationship
		err := json.NewDecoder(res.Body).Decode(&rels)
		if err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(rels) != 2 {
			t.Errorf("expected 2 relationships, got %d", len(rels))
		}
	})

	t.Run("Connection Handler", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/33683111000280/34712359000103", nil)
		w := httptest.NewRecorder()
		srv.handler(w, req)

		res := w.Result()
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Logf("failed to close response body: %v", err)
			}
		}()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}

		var rels []company.Relationship
		err := json.NewDecoder(res.Body).Decode(&rels)
		if err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(rels) != 2 {
			t.Errorf("expected path of length 2, got %d", len(rels))
		}

		if rels[0].CompanyID != "33683111000280" || rels[0].PartnerID != "3573de271293797f2abddc036be8f35e" {
			t.Errorf("unexpected first relationship: %+v", rels[0])
		}
		if rels[1].CompanyID != "34712359000103" || rels[1].PartnerID != "3573de271293797f2abddc036be8f35e" {
			t.Errorf("unexpected second relationship: %+v", rels[1])
		}
	})

	t.Run("Connection Handler Cancelled Context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest("GET", "/33683111000280/34712359000103", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		srv.handler(w, req)

		res := w.Result()
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Logf("failed to close response body: %v", err)
			}
		}()

		if res.StatusCode != http.StatusGatewayTimeout {
			t.Errorf("expected 504 Gateway Timeout, got %d", res.StatusCode)
		}
	})

	t.Run("Malformed URLs", func(t *testing.T) {
		urls := []string{
			"/123",                // too short
			"/11111111000111",     // invalid CNPJ length 14
			"/33683111000280/123", // valid id1, invalid id2
			"/123/34712359000103", // invalid id1, valid id2
			"/123/456",            // both invalid
		}
		for _, url := range urls {
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()
			srv.handler(w, req)
			res := w.Result()
			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400 Bad Request for %s, got %d", url, res.StatusCode)
			}
			if err := res.Body.Close(); err != nil {
				t.Logf("failed to close response body: %v", err)
			}
		}
	})

	t.Run("Metrics and Middleware", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/", srv.handler)
		mux.Handle("/metrics", promhttp.Handler())

		handler := bandwidthMiddleware(mux)

		req := httptest.NewRequest("GET", "/3573de271293797f2abddc036be8f35e", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		res := w.Result()
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Logf("failed to close response body: %v", err)
			}
		}()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}

		// Make a request to /metrics to retrieve them
		req = httptest.NewRequest("GET", "/metrics", nil)
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		res = w.Result()
		defer func() {
			if err := res.Body.Close(); err != nil {
				t.Logf("failed to close response body: %v", err)
			}
		}()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for metrics, got %d", res.StatusCode)
		}

		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("failed to read metrics body: %v", err)
		}

		got := string(b)
		if !strings.Contains(got, "total_requests") {
			t.Errorf("expected total_requests metric in response, got:\n%s", got)
		}
		if !strings.Contains(got, "request_duration") {
			t.Errorf("expected request_duration metric in response, got:\n%s", got)
		}
	})
}
