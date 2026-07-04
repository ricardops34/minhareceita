package api

import (
	"fmt"

	"github.com/dgraph-io/ristretto/v2"
)

const (
	minCacheSize = 1 << 3 // 8MB

	// average size of a JSON in bytes, eg:
	// SELECT AVG(octet_length(json::text)) FROM cnpj TABLESAMPLE SYSTEM(0.01);
	avgSize int64 = 1 << 11
)

type cache struct {
	r *ristretto.Cache[string, []byte]
}

func newCache(size int) (*cache, error) {
	if size == 0 {
		return nil, nil
	}
	if size < minCacheSize {
		return nil, fmt.Errorf("cache size too small, minimum is %dMB", minCacheSize)
	}
	c := int64(size) << 20 // convert MB to bytes
	r, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		MaxCost:     c,
		NumCounters: c / avgSize * 10,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &cache{r}, nil
}

func (c *cache) get(key string) ([]byte, bool) {
	return c.r.Get(key)
}

func (c *cache) set(key string, value []byte) { c.r.Set(key, value, int64(max(1, len(value)))) }
