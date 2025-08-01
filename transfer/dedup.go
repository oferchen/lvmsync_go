// transfer/dedup.go
package transfer

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"sync"

	"lvmsync_go/config"

	"github.com/bits-and-blooms/bloom/v3"
)

type DeduplicationStrategy interface {
	ShouldTransfer(offset int64, data []byte) bool
	RecordTransfer(offset int64, data []byte)
	SaveState() error
}

type ChecksumDedup struct {
	stateFile string
	hashes    map[int64][32]byte
	mu        sync.RWMutex
}

type BloomFilterDedup struct {
	filter    *bloom.BloomFilter
	stateFile string
	mu        sync.RWMutex
}

type RollingHashDedup struct {
	stateFile string
	hashes    map[int64]uint64
	mu        sync.RWMutex
}

const rollingBase uint64 = 257
const rollingMod uint64 = 2305843009213693951 // 2^61 - 1 prime

func rollingHash(data []byte) uint64 {
	var h uint64
	for _, b := range data {
		h = (h*rollingBase + uint64(b)) % rollingMod
	}
	return h
}

func NewDeduplicationStrategy(cfg *config.Config) DeduplicationStrategy {
	switch cfg.DedupStrategy {
	case "rolling_hash":
		return &RollingHashDedup{
			stateFile: cfg.DedupStateFile,
			hashes:    make(map[int64]uint64),
		}
	case "bloom":
		return &BloomFilterDedup{
			filter:    bloom.NewWithEstimates(1000000, 0.01),
			stateFile: cfg.DedupStateFile,
		}
	case "checksum":
		fallthrough
	default:
		return &ChecksumDedup{
			stateFile: cfg.DedupStateFile,
			hashes:    make(map[int64][32]byte),
		}
	}
}

func (c *ChecksumDedup) ShouldTransfer(offset int64, data []byte) bool {
	c.mu.RLock()
	prev, exists := c.hashes[offset]
	c.mu.RUnlock()

	sum := sha256.Sum256(data)
	return !exists || prev != sum
}

func (c *ChecksumDedup) RecordTransfer(offset int64, data []byte) {
	sum := sha256.Sum256(data)

	c.mu.Lock()
	c.hashes[offset] = sum
	c.mu.Unlock()
}

func (c *ChecksumDedup) SaveState() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	file, err := os.Create(c.stateFile)
	if err != nil {
		return err
	}
	defer file.Close()

	for offset, hash := range c.hashes {
		binary.Write(file, binary.LittleEndian, offset)
		file.Write(hash[:])
	}
	return nil
}

func (b *BloomFilterDedup) ShouldTransfer(offset int64, data []byte) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	h := sha256.Sum256(data)
	return !b.filter.Test(h[:])
}

func (b *BloomFilterDedup) RecordTransfer(offset int64, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	h := sha256.Sum256(data)
	b.filter.Add(h[:])
}

func (b *BloomFilterDedup) SaveState() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return nil
}

func (r *RollingHashDedup) ShouldTransfer(offset int64, data []byte) bool {
	r.mu.RLock()
	prev, exists := r.hashes[offset]
	r.mu.RUnlock()

	h := rollingHash(data)
	return !exists || prev != h
}

func (r *RollingHashDedup) RecordTransfer(offset int64, data []byte) {
	h := rollingHash(data)
	r.mu.Lock()
	r.hashes[offset] = h
	r.mu.Unlock()
}

func (r *RollingHashDedup) SaveState() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	file, err := os.Create(r.stateFile)
	if err != nil {
		return err
	}
	defer file.Close()

	for offset, hash := range r.hashes {
		if err := binary.Write(file, binary.LittleEndian, offset); err != nil {
			return err
		}
		if err := binary.Write(file, binary.LittleEndian, hash); err != nil {
			return err
		}
	}
	return nil
}
