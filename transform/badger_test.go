package transform

import (
	"strings"
	"testing"
)

func TestSerializeDeserialize(t *testing.T) {
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Errorf("expected no error opening badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	for _, tc := range []struct {
		name string
		row  []string
	}{
		{"normal", []string{"um", "dois", "três"}},
		{"empty", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := kv.serialize(tc.row)
			if err != nil {
				t.Errorf("expected no error serializing, got %s", err)
			}
			got, err := kv.deserialize(s)
			if err != nil {
				t.Errorf("expected no error deserializing, got %s", err)
			}
			for idx := range len(tc.row) {
				if got[idx] != tc.row[idx] {
					t.Errorf("expected element %d to be %s, got %s", idx+1, tc.row[idx], got[idx])
				}
			}
		})
	}
}

func TestDeserializeLargeValue(t *testing.T) {
	t.Parallel()
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error opening badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()

	for _, tc := range []struct {
		name string
		size int
	}{
		{"below pool size", defaultPoolSize - 1},
		{"exact pool size", defaultPoolSize},
		{"above pool size", defaultPoolSize + 1},
		{"way above", defaultPoolSize * 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := strings.Repeat("a", tc.size)
			row := []string{want}

			s, err := kv.serialize(row)
			if err != nil {
				t.Fatalf("expected no error serializing, got %s", err)
			}

			got, err := kv.deserialize(s)
			if err != nil {
				t.Fatalf("expected no error deserializing, got %s", err)
			}

			if len(got) != 1 {
				t.Fatalf("expected 1 element, got %d", len(got))
			}
			if len(got[0]) != tc.size {
				t.Errorf("expected element with %d bytes, got %d", tc.size, len(got[0]))
			}
			if got[0] != want {
				t.Errorf("deserialized value does not match the original")
			}
		})
	}
}

func TestPutGet(t *testing.T) {
	src := &source{prefix: "test"}
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Errorf("expected no error opening badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	for _, tc := range []struct {
		name string
		id   string
		row  []string
	}{
		{"normal", "1", []string{"um", "dois", "três"}},
		{"empty", "2", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := kv.put(src, tc.id, tc.row)
			if err != nil {
				t.Errorf("expected no error putting row, got %s", err)
			}
			if err := kv.flush(); err != nil {
				t.Errorf("expected no error flushing, got %s", err)
			}
			k := src.keyFor(tc.id)
			got, err := kv.get(k)
			if err != nil {
				t.Errorf("expected no error getting row, got %s", err)
			}
			if len(tc.row) == 0 {
				if got != nil {
					t.Errorf("expected value to be nil, got %v", got)
				}
			} else {
				for idx := range tc.row {
					if got[idx] != tc.row[idx] {
						t.Errorf("expected element %d to be %s, got %s", idx+1, tc.row[idx], got[idx])
					}
				}
			}
		})
	}
}
