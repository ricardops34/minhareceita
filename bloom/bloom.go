// Package bloom provides a CNPJ presence filter for fast "not found" checks.
package bloom

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
)

const (
	numHashFuncs = 7

	// 32MB filter for ~60M elements with ~10% false positive rate
	bloomSize = 1 << 28

	// how many CNPJs to request to the database per page to initialize the filter
	pageSize = 1 << 18

	// golden ratio for double hashing without allocations
	goldenRatio = 0x9e3779b97f4a7c15
)

type database interface {
	AllCompanies(ctx context.Context, cursor *string, limit uint32) ([]string, *string, error)
}

type Filter struct {
	db   database
	bits []uint64
	ok   atomic.Bool
	pool sync.Pool
}

func New(db database) *Filter {
	s := (bloomSize + 63) / 64 //  ceil(bloomSize / 64) uint64s
	return &Filter{
		db:   db,
		bits: make([]uint64, s),
		pool: sync.Pool{
			New: func() any {
				s := make([]uint, numHashFuncs)
				return &s
			},
		},
	}
}

func (f *Filter) bitPositions(id string, pos []uint) {
	h1 := xxhash.Sum64String(id)
	h2 := h1 * goldenRatio
	for i := range numHashFuncs {
		h := h1 + uint64(i)*h2
		pos[i] = uint(h % uint64(bloomSize))
	}
}

func (f *Filter) add(id string, pos []uint) {
	f.bitPositions(id, pos)
	for _, p := range pos {
		idx := p / 64
		bit := p % 64
		f.bits[idx] |= 1 << bit
	}
}

func (f *Filter) Initialize(ctx context.Context) error {
	slog.Info("Building CNPJ presence filter…")
	ini := time.Now()

	pos := make([]uint, numHashFuncs)
	var ids []string
	var c *string
	var err error
	var t uint64
	for {
		b := time.Now()
		ids, c, err = f.db.AllCompanies(ctx, c, pageSize)
		if err != nil {
			slog.Error("Failed to build CNPJ presence filter", "error", err)
			return fmt.Errorf("could not build bloom filter: %w", err)
		}
		if len(ids) == 0 {
			if c == nil {
				break
			}
			continue
		}
		for _, id := range ids {
			f.add(id, pos)
			t++
		}
		slog.Debug(
			"Building CNPJ presence filter",
			"count", t,
			"batch_duration_ms", time.Since(b),
			"total_duration", time.Since(ini),
		)
		if c == nil {
			break
		}
	}

	f.ok.Store(true)
	slog.Info("CNPJ presence filter ready", "count", t, "duration", time.Since(ini))
	return nil
}

func (f *Filter) Ready() bool { return f.ok.Load() }

func (f *Filter) Exists(id string) (bool, error) {
	if !f.ok.Load() {
		return true, errors.New("bloom filter not ready")
	}

	pos := f.pool.Get().(*[]uint)
	defer f.pool.Put(pos)

	f.bitPositions(id, *pos)
	for _, p := range *pos {
		idx := p / 64
		bit := p % 64
		if (f.bits[idx] & (1 << bit)) == 0 {
			return false, nil
		}
	}
	return true, nil
}
