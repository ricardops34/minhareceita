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
)

var postgresDefaultIndexes = []string{"cnpj_pkey", "cnpj_id"}

func setUpPostgres(id string, c company.Company) (*PostgreSQL, error) {
	u := os.Getenv("TEST_POSTGRES_URL")
	if u == "" {
		return nil, fmt.Errorf("expected a posgres uri at TEST_POSTGRES_URL, found nothing")
	}
	var db PostgreSQL
	var err error
	for range dbRetryAttempts {
		db, err = NewPostgreSQL(&Args{URI: u, PostgresSchema: "public", MaxConns: maxConnsDefault})
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("expected no error connecting to postgres, got %w", err)
	}
	if err := db.Drop(); err != nil {
		return nil, fmt.Errorf("expected no error dropping the tables, got %w", err)
	}
	if err := db.Create(); err != nil {
		return nil, fmt.Errorf("expected no error creating the tables, got %w", err)
	}
	if err := db.PreLoad(); err != nil {
		return nil, fmt.Errorf("expected no error pre load on postgres, got %w", err)
	}
	c.CNPJ = id
	if err := db.CreateCompanies(context.Background(), []company.Company{c}); err != nil {
		return nil, fmt.Errorf("expected no error saving a company to postgres, got %w", err)
	}
	if err := db.PostLoad(); err != nil {
		return nil, fmt.Errorf("expected no error post load on postgres, got %w", err)
	}
	return &db, nil
}

func listIndexesPostgres(t *testing.T, pg *PostgreSQL) []string {
	q := `
		SELECT indexname
		FROM pg_indexes
		WHERE tablename = $1 AND schemaname = 'public'
	`
	c := context.Background()
	r, err := pg.pool.Query(c, q, pg.CompanyTableName)
	if err != nil {
		t.Errorf("expected no errors checking index list, got %s", err)
		return nil
	}
	defer r.Close()
	var i []string
	for r.Next() {
		var iname string
		if err := r.Scan(&iname); err != nil {
			t.Errorf("expected no error scanning index name, got %s", err)
			continue
		}
		if !slices.Contains(postgresDefaultIndexes, iname) {
			i = append(i, strings.TrimPrefix(iname, "idx_json."))
		}
	}
	return i
}

func TestPostgresCreateIndexes(t *testing.T) {
	id := "33683111000280"
	b, err := os.ReadFile(filepath.Join("..", "testdata", "response.json"))
	if err != nil {
		t.Error("error reading company JSON file")
	}
	var c company.Company
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("expected no error unmarshalling test company, got %s", err)
	}
	pg, err := setUpPostgres(id, c)
	if err != nil {
		t.Errorf("expected no error setting up postgres, got %s", err)
		return
	}
	defer func() {
		if err := pg.Drop(); err != nil {
			t.Errorf("expected no error dropping the tables, got %s", err)
		}
		pg.Close()
	}()
	i := []string{"qsa.nome_socio"}
	if err := pg.CreateExtraIndexes(i); err != nil {
		t.Errorf("expected no errors running extra indexes, got %s", err)
	}
	testutils.AssertArraysHaveSameItems(t, i, listIndexesPostgres(t, pg))
}

func TestPostgresAllCompanies(t *testing.T) {
	id := "33683111000280"
	b, err := os.ReadFile(filepath.Join("..", "testdata", "response.json"))
	if err != nil {
		t.Error("error reading company JSON file")
	}
	var c company.Company
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("expected no error unmarshalling test company, got %s", err)
	}
	pg, err := setUpPostgres(id, c)
	if err != nil {
		t.Errorf("expected no error setting up postgres, got %s", err)
		return
	}
	defer func() {
		if err := pg.Drop(); err != nil {
			t.Errorf("expected no error dropping the tables, got %s", err)
		}
		pg.Close()
	}()

	id2 := "00000000000000"
	id3 := "11111111111111"
	c2 := c
	c2.CNPJ = id2

	c3 := c
	c3.CNPJ = id3

	if err := pg.CreateCompanies(context.Background(), []company.Company{c2, c3}); err != nil {
		t.Errorf("expected no error saving additional companies to postgres, got %s", err)
		return
	}

	ids, cur, err := pg.AllCompanies(context.Background(), nil, 2)
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
		ids2, cur2, err := pg.AllCompanies(context.Background(), cur, 2)
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

	not := "not-a-number"
	_, _, err = pg.AllCompanies(context.Background(), &not, 2)
	if err == nil {
		t.Error("expected error for invalid cursor")
	}
}
