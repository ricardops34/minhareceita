package db

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"codeberg.org/cuducos/minha-receita/company"
	"codeberg.org/cuducos/minha-receita/testutils"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var mongoDefaultIndexes = []string{"_id_", "id_1"}

func setUpMongo(id string, c company.Company) (*MongoDB, error) {
	u := os.Getenv("TEST_MONGODB_URL")
	if u == "" {
		return nil, fmt.Errorf("expected a mongodb uri at TEST_MONGODB_URL, found nothing")
	}
	db, err := NewMongoDB(&Args{URI: u})
	if err != nil {
		return nil, fmt.Errorf("expected no error connecting to mongodb, got %s", err)
	}
	if err := db.Drop(); err != nil {
		return nil, fmt.Errorf("expected no error dropping the collections, got %s", err)
	}
	if err := db.Create(); err != nil {
		return nil, fmt.Errorf("expected no error creating the collections, got %s", err)
	}
	if err := db.PreLoad(); err != nil {
		return nil, fmt.Errorf("expected no error pre load on mongo, got %w", err)
	}
	c.CNPJ = id
	if err := db.CreateCompanies(context.Background(), []company.Company{c}); err != nil {
		return nil, fmt.Errorf("expected no error saving a company to mongo, got %s", err)
	}
	if err := db.PostLoad(); err != nil {
		return nil, fmt.Errorf("expected no error post load on mongo, got %s", err)
	}
	return &db, nil
}

func listIndexesMongo(t *testing.T, db *MongoDB) []string {
	c, err := db.db.Collection(companyTableName).Indexes().List(context.Background())
	if err != nil {
		t.Errorf("expected no errors checking index list, got %s", err)
	}
	defer func() {
		if err := c.Close(context.Background()); err != nil {
			t.Errorf("expected no error closing the connection, got %s", err)
		}
	}()
	var i []string
	for c.Next(context.Background()) {
		var idx bson.M
		if err := c.Decode(&idx); err != nil {
			t.Errorf("expected no error decoding index, got %s", err)
		}
		n, ok := idx["name"].(string)
		if ok && !slices.Contains(mongoDefaultIndexes, n) {
			i = append(i, strings.TrimPrefix(n, "idx_json."))
		}
	}
	return i
}

func TestMongoCreateIndexes(t *testing.T) {
	id := "33683111000280"
	b, err := os.ReadFile(filepath.Join("..", "testdata", "response.json"))
	if err != nil {
		t.Error("error reading company JSON file")
	}
	var c company.Company
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("expected no error unmarshalling test company, got %s", err)
	}
	m, err := setUpMongo(id, c)
	if err != nil {
		t.Errorf("expected no error setting up postgres, got %s", err)
		return
	}
	defer func() {
		if err := m.Drop(); err != nil {
			t.Errorf("expected no error dropping the tables, got %s", err)
		}
		m.Close()
	}()
	i := []string{"qsa.nome_socio"}
	if err := m.CreateExtraIndexes(i); err != nil {
		t.Errorf("expected no errors running extra indexes, got %s", err)
	}
	testutils.AssertArraysHaveSameItems(t, i, listIndexesMongo(t, m))
}

func TestMongoAllCompanies(t *testing.T) {
	id := "33683111000280"
	b, err := os.ReadFile(filepath.Join("..", "testdata", "response.json"))
	if err != nil {
		t.Error("error reading company JSON file")
	}
	var c company.Company
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("expected no error unmarshalling test company, got %s", err)
	}
	m, err := setUpMongo(id, c)
	if err != nil {
		t.Errorf("expected no error setting up mongo, got %s", err)
		return
	}
	defer func() {
		if err := m.Drop(); err != nil {
			t.Errorf("expected no error dropping the tables, got %s", err)
		}
		m.Close()
	}()

	id2 := "00000000000000"
	id3 := "11111111111111"
	c2 := c
	c2.CNPJ = id2

	c3 := c
	c3.CNPJ = id3

	if err := m.CreateCompanies(context.Background(), []company.Company{c2, c3}); err != nil {
		t.Errorf("expected no error saving additional companies to mongo, got %s", err)
		return
	}

	ids, cur, err := m.AllCompanies(context.Background(), nil, 2)
	if err != nil {
		t.Errorf("expected no error getting first page, got %s", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}
	if cur == nil {
		t.Error("expected cursor for next page")
	}

	if cur != nil {
		ids2, cur2, err := m.AllCompanies(context.Background(), cur, 2)
		if err != nil {
			t.Errorf("expected no error getting second page, got %s", err)
		}
		if len(ids2) != 1 {
			t.Errorf("expected 1 ID on second page, got %d", len(ids2))
		}
		if cur2 != nil {
			t.Error("expected nil cursor at the end")
		}
	}

	not := "not-a-valid-objectid"
	_, _, err = m.AllCompanies(context.Background(), &not, 2)
	if err == nil {
		t.Error("expected error for invalid cursor")
	}
}

func TestMongoGetNeighbors(t *testing.T) {
	id := "33683111000280"
	b, err := os.ReadFile(filepath.Join("..", "testdata", "response.json"))
	if err != nil {
		t.Error("error reading company JSON file")
	}
	var c company.Company
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("expected no error unmarshalling test company, got %s", err)
	}
	m, err := setUpMongo(id, c)
	if err != nil {
		t.Errorf("expected no error setting up mongo, got %s", err)
		return
	}
	defer func() {
		if err := m.Drop(); err != nil {
			t.Errorf("expected no error dropping the collections, got %s", err)
		}
		m.Close()
	}()

	if err := m.CreateGraphTable(); err != nil {
		t.Errorf("expected no error creating graph table, got %s", err)
	}

	var ns []GraphEdge
	t.Run("from company perspective", func(t *testing.T) {
		var err error
		ns, err = m.GetRelated(context.Background(), id)
		if err != nil {
			t.Errorf("expected no error getting relations, got %s", err)
		}
		if len(ns) == 0 {
			t.Error("expected at least one relation for the company")
		}
	})

	t.Run("from partner perspective", func(t *testing.T) {
		p := ns[0].PartnerID
		ns2, err := m.GetRelated(context.Background(), p)
		if err != nil {
			t.Errorf("expected no error getting relations for partner, got %s", err)
		}
		ok := false
		for _, n := range ns2 {
			if n.CompanyID == id {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("expected to find company %s as a relation of partner %s", id, p)
		}
	})
}
