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
	"time"

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
	var db MongoDB
	var err error
	for range dbRetryAttempts {
		db, err = NewMongoDB(&Args{URI: u})
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
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

func TestMongoStreamRelationships(t *testing.T) {
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

	ctx := context.Background()

	n, err := m.RelationshipCount(ctx)
	if err != nil {
		t.Fatalf("expected no error getting relationship count, got %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 relationship, got %d", n)
	}

	var rels []Relationship
	err = m.StreamRelationships(ctx, func(r Relationship) error {
		rels = append(rels, r)
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error streaming relationships, got %v", err)
	}

	if len(rels) != 1 {
		t.Fatalf("expected 1 streamed relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.CompanyID != id {
		t.Errorf("expected CompanyID to be %s, got %s", id, rel.CompanyID)
	}
	if rel.CompanyName != "OPEN KNOWLEDGE BRASIL" {
		t.Errorf("expected CompanyName to be 'OPEN KNOWLEDGE BRASIL', got %s", rel.CompanyName)
	}
	if rel.PartnerName != "HAYDEE SVAB" {
		t.Errorf("expected PartnerName to be 'HAYDEE SVAB', got %s", rel.PartnerName)
	}
	if rel.PartnerCPF != "***112108**" {
		t.Errorf("expected PartnerCPF to be '***112108**', got %s", rel.PartnerCPF)
	}
}
