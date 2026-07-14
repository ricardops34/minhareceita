package transform

import (
	"sync"
	"testing"
)

func TestHashSet(t *testing.T) {
	t.Parallel()

	t.Run("simple", func(t *testing.T) {
		h := newHashSet(1)
		t.Parallel()
		if h.has(123) {
			t.Error("expected 123 to not exist initially")
		}
		h.add(123)
		if !h.has(123) {
			t.Error("expected 123 to exist after being added")
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		h := newHashSet(1 << 10)
		var g sync.WaitGroup
		for i := range 1000 {
			g.Go(func() {
				h.add(uint64(i))
			})
		}
		g.Wait()
		for i := range 1000 {
			if !h.has(uint64(i)) {
				t.Errorf("expected %d to exist from concurrent inserts", i)
			}
		}
	})
}
