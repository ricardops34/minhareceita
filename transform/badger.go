package transform

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/ristretto/v2"
)

// As of 2025-11 the longest sequence we've got was 257, so setting it to 512 to
// have some room — maybe this could be set from a CLI flag to avoid recompiling
// when source data changes and needs more space.
const defaultPoolSize = 512

type kv struct {
	db    *badger.DB
	wb    *badger.WriteBatch
	pool  sync.Pool
	cache *ristretto.Cache[uint64, []string]
}

func (kv *kv) serialize(row []string) ([]byte, error) {
	var b []byte
	var err error
	for _, v := range row {
		s := uint32(len(v)) // used to deserialize later on
		b, err = binary.Append(b, binary.LittleEndian, s)
		if err != nil {
			return nil, err
		}
		b = append(b, v...)
	}
	return b, nil
}

func (kv *kv) deserialize(val []byte) ([]string, error) {
	if val == nil {
		return nil, nil
	}
	var out []string
	r := bytes.NewReader(val)
	for r.Len() > 0 {
		err := func() error {
			var s uint32
			if err := binary.Read(r, binary.LittleEndian, &s); err != nil {
				return fmt.Errorf("error reading size: %w", err)
			}
			b := kv.pool.Get().(*[]byte)
			defer kv.pool.Put(b)
			if cap(*b) < int(s) {
				*b = make([]byte, s)
			} else {
				*b = (*b)[:s]
			}
			n, err := io.ReadFull(r, *b)
			if err != nil {
				return fmt.Errorf("could not deserialize value: %w", err)
			}
			if n != int(s) {
				return fmt.Errorf("expected to read %d bytes, got %d", s, n)
			}
			out = append(out, string(*b))
			return nil
		}()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (kv *kv) put(src *source, id string, row []string) error {
	if len(row) == 0 {
		return nil
	}
	key := src.keyFor(id)
	val, err := kv.serialize(row)
	if err != nil {
		return fmt.Errorf("could not serialize row %v: %w", row, err)
	}
	return kv.wb.Set(key, val)
}

func (kv *kv) flush() error {
	if err := kv.wb.Flush(); err != nil {
		return err
	}
	kv.wb = kv.db.NewWriteBatch()
	return nil
}

func (kv *kv) get(k []byte) ([]string, error) {
	h := xxhash.Sum64(k)
	if out, ok := kv.cache.Get(h); ok {
		return out, nil
	}

	val := kv.pool.Get().(*[]byte)
	*val = (*val)[:0]
	defer kv.pool.Put(val)
	err := kv.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(k)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return fmt.Errorf("could not get key: %w", err)
		}
		*val, err = item.ValueCopy(*val)
		if err != nil {
			return fmt.Errorf("could not read value: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("could not get key %s: %w", string(k), err)
	}
	out, err := kv.deserialize(*val)
	if err != nil {
		return nil, err
	}

	var t int64
	for _, v := range out {
		t += int64(len(v))
	}
	kv.cache.Set(h, out, t)
	return out, nil
}

func (kv *kv) getPrefix(k []byte) ([][]string, error) {
	vs := [][]string{}
	err := kv.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(k); it.ValidForPrefix(k); it.Next() {
			i := it.Item()
			err := i.Value(func(b []byte) error {
				v, err := kv.deserialize(b)
				if err != nil {
					return fmt.Errorf("could not deserialize %s: %w", string(i.Key()), err)
				}
				vs = append(vs, v)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return vs, nil
}

type noLogger struct{}

func (*noLogger) Errorf(string, ...any)   {}
func (*noLogger) Warningf(string, ...any) {}
func (*noLogger) Infof(string, ...any)    {}
func (*noLogger) Debugf(string, ...any)   {}

func newBadger(dir string, ro bool) (*kv, error) {
	opt := badger.DefaultOptions(dir).WithReadOnly(ro).WithBypassLockGuard(true).WithDetectConflicts(false)
	slog.Debug("Creating temporary key-value storage", "path", dir)
	if os.Getenv("DEBUG") == "" {
		opt = opt.WithLogger(&noLogger{})
	}
	db, err := badger.Open(opt)
	if err != nil {
		return nil, fmt.Errorf("could not open badger at %s: %w", dir, err)
	}
	kv := &kv{
		db: db,
		wb: db.NewWriteBatch(),
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, defaultPoolSize)
				return &b
			},
		},
	}
	kv.cache, err = ristretto.NewCache(&ristretto.Config[uint64, []string]{
		MaxCost:     1 << 30,
		NumCounters: 1 << 23,
		BufferItems: 1 << 6,
	})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("could not create cache: %w", err), db.Close())
	}
	return kv, nil
}
