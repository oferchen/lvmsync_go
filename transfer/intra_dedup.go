package transfer

import (
	"bytes"
	"hash/maphash"
	"sync"
)

// chunkCache provides a bounded rolling-hash map for intra-run deduplication.
// It stores recent chunk hashes with FIFO eviction to bound memory usage.
type chunkCache struct {
	mu    sync.Mutex
	seed  maphash.Seed
	max   int
	order []uint64
	data  map[uint64][]byte
	next  int
}

// newChunkCache initializes a cache that keeps up to max entries.
func newChunkCache(max int) *chunkCache {
	return &chunkCache{
		seed:  maphash.MakeSeed(),
		max:   max,
		data:  make(map[uint64][]byte, max),
		order: make([]uint64, 0, max),
	}
}

// Seen reports whether b has been observed before. New chunks are inserted
// and may evict the oldest entry when the cache is full.
func (c *chunkCache) Seen(b []byte) bool {
	h := hashChunk(c.seed, b)
	c.mu.Lock()
	defer c.mu.Unlock()
	if prev, ok := c.data[h]; ok && bytes.Equal(prev, b) {
		return true
	}
	// Insert copy of b.
	buf := make([]byte, len(b))
	copy(buf, b)
	if len(c.order) < c.max {
		c.order = append(c.order, h)
	} else {
		old := c.order[c.next]
		delete(c.data, old)
		c.order[c.next] = h
		c.next = (c.next + 1) % c.max
	}
	c.data[h] = buf
	return false
}

func hashChunk(seed maphash.Seed, b []byte) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	h.Write(b)
	return h.Sum64()
}
