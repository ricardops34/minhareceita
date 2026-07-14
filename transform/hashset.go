package transform

import (
	"sync"
)

const nShards = 256

type shard struct {
	mu sync.Mutex
	m  map[uint64]struct{}
}

// hashSet is a thread-safe, sharded set for uint64.
// It is designed for low lock contention under high concurrency.
type hashSet struct {
	shards [nShards]shard
}

func newHashSet(n int) *hashSet {
	h := &hashSet{}
	for i := range h.shards {
		h.shards[i].m = make(map[uint64]struct{}, n/nShards)
	}
	return h
}

func (h *hashSet) has(hash uint64) bool {
	s := &h.shards[hash%256]
	s.mu.Lock()
	_, ok := s.m[hash]
	s.mu.Unlock()
	return ok
}

func (h *hashSet) add(hash uint64) {
	s := &h.shards[hash%256]
	s.mu.Lock()
	s.m[hash] = struct{}{}
	s.mu.Unlock()
}
