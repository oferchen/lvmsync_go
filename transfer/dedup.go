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

func NewDeduplicationStrategy(cfg *config.Config) DeduplicationStrategy {
	switch cfg.DedupStrategy {
	case "bloom":
		return &BloomFilterDedup{
			filter:    bloom.NewWithEstimates(1000000, 0.01),
			stateFile: cfg.DedupStateFile,
		}
	default:
		return &ChecksumDedup{
			stateFile: cfg.DedupStateFile,
			hashes:    make(map[int64][32]byte),
		}
	}
}

func (c *ChecksumDedup) ShouldTransfer(offset int64, data []byte) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sum := sha256.Sum256(data)
	prev, exists := c.hashes[offset]
	return !exists || prev != sum
}

func (c *ChecksumDedup) RecordTransfer(offset int64, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashes[offset] = sha256.Sum256(data)
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
