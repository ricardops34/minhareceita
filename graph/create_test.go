package graph

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/cuducos/minha-receita/db"
)

type mockStreamer struct {
	relationships []db.Relationship
}

func (m *mockStreamer) StreamRelationships(ctx context.Context, callback func(db.Relationship) error) error {
	for _, r := range m.relationships {
		if err := callback(r); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockStreamer) RelationshipCount(ctx context.Context) (int64, error) {
	return int64(len(m.relationships)), nil
}

func TestCreate(t *testing.T) {
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

	data := []db.Relationship{
		{
			CompanyID:   "11111111000111",
			CompanyName: "Company A",
			PartnerID:   "22222222222",
			PartnerName: "Partner B",
			PartnerCPF:  "22222222222",
			PartnerType: 2,
		},
	}

	s := &mockStreamer{relationships: data}

	err = Create(context.Background(), s, int64(len(data)), path, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("graph badger directory not created")
	}
}
