package api

import (
	"fmt"
	"testing"
)

func TestCacheSetEmptyValueIsRetained(t *testing.T) {
	t.Parallel()
	for _, v := range [][]byte{{}, nil} {
		t.Run(fmt.Sprintf("%v as empty value", v), func(t *testing.T) {
			t.Parallel()

			c, err := newCache(minCacheSize)
			if err != nil {
				t.Fatalf("could not create cache: %v", err)
			}

			c.set("19131243000197", v)
			c.r.Wait()

			s, ok := c.get("19131243000197")
			if !ok {
				t.Fatal("expected empty (negative) cache entry to be retained, but it was not found")
			}
			if len(s) != 0 {
				t.Errorf("expected an empty value, but got %q", s)
			}
		})
	}
}
