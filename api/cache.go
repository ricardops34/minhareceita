package api

import (
	"log/slog"

	"github.com/dgraph-io/ristretto/v2"
)

type cache struct {
	r *ristretto.Cache[string, string]
}

func newCache() *cache {
	r, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 200_000, // 10x the expected max items (20k)
		MaxCost:     1 << 25, // 32 MB
		BufferItems: 64,
	})
	if err != nil {
		slog.Error("could not create cache, running without it", "error", err)
		return nil
	}
	return &cache{r}
}

func (c *cache) get(key string) (string, bool) {
	return c.r.Get(key)
}

func (c *cache) set(key, value string) {
	c.r.Set(key, value, int64(len(value)))
}
