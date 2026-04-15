// Package bloom provides a CNPJ presence filter for fast "not found" checks.
package bloom

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
)

const (
	numHashFuncs = 7

	// how many CNPJs to request to the database per page durint the
	// initialization
	pageSize = 1 << 18

	// golden ratio for double hashing without allocations
	goldenRatio = 0x9e3779b97f4a7c15
)

type database interface {
	AllCompanies(ctx context.Context, cursor *string, limit uint32) ([]string, *string, error)
}

type Filter struct {
	db   database
	size uint64 // in bits (not bytes)
	bits []uint64
	ok   atomic.Bool
	pool sync.Pool
}

// New creates a new bloom filter with the given size in megabytes. The size is
// expected in MB.
func New(db database, size int) *Filter {
	s := uint64(size) << 23 // MB to bits
	b := (s + 63) / 64      // ceil(s / 64) uint64s
	f := &Filter{
		db:   db,
		size: s,
		bits: make([]uint64, b),
		pool: sync.Pool{
			New: func() any {
				s := make([]uint, numHashFuncs)
				return &s
			},
		},
	}
	return f
}

func (f *Filter) bitPositions(id string, pos []uint) {
	h1 := xxhash.Sum64String(id)
	h2 := h1 * goldenRatio
	for i := range numHashFuncs {
		h := h1 + uint64(i)*h2
		pos[i] = uint(h % f.size)
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
	slog.Info("CNPJ presence filter ready",
		"count", t,
		"duration", time.Since(ini),
		"size_mb", f.size>>23, // Convert bits back to MB
		"error_rate", fmt.Sprintf("%.6f%%", f.errorRate(t)*100),
	)
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

func (f *Filter) errorRate(total uint64) float64 {
	if f.size == 0 {
		return 1.0
	}
	h := float64(numHashFuncs)
	return math.Pow(1.0-math.Exp(-h*float64(total)/float64(f.size)), h)
}
