package bloom

import (
	"context"
	"math"
	"testing"
)

type mockDB struct {
	ids []string
}

func (m *mockDB) AllCompanies(ctx context.Context, c *string, l uint32) ([]string, *string, error) {
	s := 0
	if c != nil {
		for i, id := range m.ids {
			if id == *c {
				s = i + 1
				break
			}
		}
	}

	e := min(s+int(l), len(m.ids))

	r := m.ids[s:e]
	var n *string
	if e < len(m.ids) {
		n = &m.ids[e-1]
	}

	return r, n, nil
}

func TestBloomFilter(t *testing.T) {
	ids := make([]string, 1000)
	for i := range ids {
		b := make([]byte, 14)
		for j := range b {
			b[j] = '0' + byte((i+j)%10)
		}
		ids[i] = string(b)
	}

	m := &mockDB{ids: ids}
	f := New(m, 32)

	ctx := context.Background()
	if err := f.Initialize(ctx); err != nil {
		t.Fatalf("expected no error initializing bloom filter, got %s", err)
	}

	if !f.Ready() {
		t.Error("expected bloom filter to be ready after initialization")
	}

	for _, id := range ids {
		ok, err := f.Exists(id)
		if err != nil {
			t.Errorf("expected no error checking CNPJ %s, got %s", id, err)
		}
		if !ok {
			t.Errorf("expected CNPJ %s to exist in bloom filter", id)
		}
	}

	ne := "99999999999999"
	ok, err := f.Exists(ne)
	if err != nil {
		t.Errorf("expected no error checking non-existent CNPJ, got %s", err)
	}
	t.Logf("non-existent CNPJ check: %v (false positive is acceptable)", ok)
}

func TestBloomFilterNotReady(t *testing.T) {
	m := &mockDB{ids: []string{}}
	f := New(m, 32)

	if f.Ready() {
		t.Error("expected bloom filter not to be ready before initialization")
	}

	_, err := f.Exists("12345678901234")
	if err == nil {
		t.Error("expected error when checking bloom filter that is not ready")
	}
}

func TestBloomFilterEmpty(t *testing.T) {
	m := &mockDB{ids: []string{}}
	f := New(m, 32)

	ctx := context.Background()
	if err := f.Initialize(ctx); err != nil {
		t.Fatalf("expected no error initializing empty bloom filter, got %s", err)
	}

	if !f.Ready() {
		t.Error("expected bloom filter to be ready even when empty")
	}

	ok, err := f.Exists("12345678901234")
	if err != nil {
		t.Errorf("expected no error checking CNPJ in empty filter, got %s", err)
	}
	if ok {
		t.Error("expected empty bloom filter to always return false")
	}
}

func TestErrorRate(t *testing.T) {
	m := &mockDB{ids: []string{}}

	t.Run("zero elements yields zero error rate", func(t *testing.T) {
		f := New(m, 32)
		if rate := f.errorRate(0); rate != 0.0 {
			t.Errorf("expected 0.0, got %v", rate)
		}
	})

	tt := []struct {
		name  string
		size  int
		total uint64
		exp   float64
		ep    float64
	}{
		{"60M elements in 32MB", 32, 60_000_000, 0.1935, 0.0001},
		{"70M elements in 32MB", 32, 70_000_000, 0.2923, 0.0001},
		{"70M elements in 64MB", 64, 70_000_000, 0.0275, 0.0001},
		{"70M elements in 16MB", 16, 70_000_000, 0.8318, 0.0001},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			f := New(m, tc.size)
			rate := f.errorRate(tc.total)
			if math.Abs(rate-tc.exp) > tc.ep {
				t.Errorf("want %.4f, got %.4f", tc.exp, rate)
			}
		})
	}
}
