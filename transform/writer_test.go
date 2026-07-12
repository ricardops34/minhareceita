package transform

import (
	"context"
	"sync"
	"testing"

	"codeberg.org/cuducos/minha-receita/company"
)

type testDB struct {
	lock sync.Mutex
	data map[string]company.Company
	meta map[string]string
}

func (db *testDB) PreLoad() error {
	db.lock.Lock()
	defer db.lock.Unlock()
	db.data = make(map[string]company.Company)
	db.meta = make(map[string]string)
	return nil
}

func (db *testDB) CreateCompanies(_ context.Context, companies []company.Company) error {
	db.lock.Lock()
	defer db.lock.Unlock()
	for _, c := range companies {
		db.data[c.CNPJ] = c
	}
	return nil
}

func (db *testDB) PostLoad() error {
	return nil
}

func (db *testDB) CreateExtraIndexes(indexes []string) error {
	return nil
}

func (db *testDB) MetaSave(key, value string) error {
	db.lock.Lock()
	defer db.lock.Unlock()
	db.meta[key] = value
	return nil
}

func TestWriteJSONs(t *testing.T) {
	ctx := context.Background()
	srcs := sources()
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error creating badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	loadAllTestSources(t, kv)
	db := &testDB{}
	if err := db.PreLoad(); err != nil {
		t.Fatalf("expected no error calling PreLoad, got %s", err)
	}
	src := newCompanySrc("Estabelecimentos", ';', false, false)
	w, err := newWriter(db, kv, srcs, 8192, false, testdataDir, src)
	if err != nil {
		t.Fatalf("expected no error creating writer, got %s", err)
	}
	defer w.Close()
	err = w.write(ctx)
	if err != nil {
		t.Fatalf("expected no error processing test data, got %s", err)
	}
	db.lock.Lock()
	defer db.lock.Unlock()
	if len(db.data) != 1 {
		t.Errorf("expected 1 company to be persisted, got %d", len(db.data))
	}
	exp := "33683111000280"
	if _, ok := db.data[exp]; !ok {
		t.Errorf("expected CNPJ %s to be persisted, got nil", exp)
	}
}
