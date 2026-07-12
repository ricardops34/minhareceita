package graph

import (
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/cuducos/minha-receita/company"
)

func TestWriter(t *testing.T) {
	tmp, err := os.MkdirTemp("", "create_test_*")
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
			CompanyID:   "11111111000111",
			CompanyName: "Company A",
			PartnerID:   "22222222222",
			PartnerName: "Partner B",
			PartnerCPF:  "22222222222",
			PartnerType: 2,
		},
	}

	w, err := NewWriter(path)
	if err != nil {
		t.Fatalf("writer failed: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Errorf("expected no error closing writer, got %q", err)
		}
	}()

	for _, r := range data {
		if err := w.Save(&r); err != nil {
			t.Errorf("expected no error saving relationship, got %q", err)
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("graph badger directory not created")
	}
}
