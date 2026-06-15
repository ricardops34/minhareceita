package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"codeberg.org/cuducos/minha-receita/testutils"
)

var postgresDefaultIndexes = []string{"cnpj_pkey", "cnpj_id"}

func setUpPostgres(id, c string) (*PostgreSQL, error) {
	u := os.Getenv("TEST_POSTGRES_URL")
	if u == "" {
		return nil, fmt.Errorf("expected a posgres uri at TEST_POSTGRES_URL, found nothing")
	}
	db, err := NewPostgreSQL(u, "public", false)
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
	if err := db.CreateCompanies(context.Background(), [][]string{{id, c}}); err != nil {
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
	c := string(b)
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
	c := string(b)
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
	c2 := strings.Replace(c, id, id2, 1)
	c3 := strings.Replace(c, id, id3, 1)

	if err := pg.CreateCompanies(context.Background(), [][]string{{id2, c2}, {id3, c3}}); err != nil {
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

func TestPostgresGetNeighbors(t *testing.T) {
	id := "33683111000280"
	b, err := os.ReadFile(filepath.Join("..", "testdata", "response.json"))
	if err != nil {
		t.Error("error reading company JSON file")
	}
	c := string(b)
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

	if err := pg.CreateGraphTable(); err != nil {
		t.Errorf("expected no error creating graph table, got %s", err)
	}

	var ns []GraphEdge
	t.Run("from company perspective", func(t *testing.T) {
		var err error
		ns, err = pg.GetRelated(context.Background(), id)
		if err != nil {
			t.Errorf("expected no error getting relations, got %s", err)
		}
		if len(ns) == 0 {
			t.Error("expected at least one relation for the company")
		}
	})

	t.Run("from partner perspective", func(t *testing.T) {
		p := ns[0].PartnerID
		ns2, err := pg.GetRelated(context.Background(), p)
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
