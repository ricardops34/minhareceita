package transform

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/dgraph-io/badger/v4"
)

type seenDB struct {
	db *badger.DB
}

func newSeenDB(dir string) (*seenDB, error) {
	opt := badger.DefaultOptions(dir).
		WithSyncWrites(false).
		WithBlockCacheSize(1 << 24).
		WithIndexCacheSize(0).
		WithMemTableSize(1 << 25)
	if os.Getenv("DEBUG") == "" {
		opt = opt.WithLogger(&noLogger{})
	}
	db, err := badger.Open(opt)
	if err != nil {
		return nil, fmt.Errorf("could not open seen badger at %s: %w", dir, err)
	}
	return &seenDB{db: db}, nil
}

func (db *seenDB) check(key string) (bool, error) {
	var ok bool
	err := db.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(key))
		if err == nil {
			ok = true
			return nil
		}
		if err != badger.ErrKeyNotFound {
			return err
		}
		return txn.Set([]byte(key), nil)
	})
	if err == badger.ErrConflict {
		return true, nil // set by a concurrent txn
	}
	if err != nil {
		return false, fmt.Errorf("could not update seen status for %s: %w", key, err)
	}
	return ok, nil
}

func (db *seenDB) close() {
	if err := db.db.Close(); err != nil {
		slog.Warn("could not close seen badger", "error", err)
	}
}
