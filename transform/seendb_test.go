package transform

import (
	"testing"
)

func TestSeenDB(t *testing.T) {
	t.Parallel()

	t.Run("Check", func(t *testing.T) {
		tmp := t.TempDir()
		db, err := newSeenDB(tmp)
		if err != nil {
			t.Fatalf("could not create seenDB: %v", err)
		}
		defer db.close()
		k := "19.131.243/0001-97"
		ok, err := db.check(k)
		if err != nil {
			t.Errorf("expected no error on first check, got %v", err)
		}
		if ok {
			t.Error("expected CNPJ to not be seen on first check")
		}
		ok, err = db.check(k)
		if err != nil {
			t.Errorf("expected no error on second check, got %v", err)
		}
		if !ok {
			t.Error("expected CNPJ to be seen on second check")
		}
		ok, err = db.check("00.000.000/0001-91")
		if err != nil {
			t.Errorf("expected no error on check of new CNPJ, got %v", err)
		}
		if ok {
			t.Error("expected new CNPJ to not be seen")
		}
	})
}
