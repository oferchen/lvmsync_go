// transfer/dedup.go
package transfer

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"os"
	"sync"

	"lvmsync_go/config"

	"github.com/bits-and-blooms/bloom/v3"
)

type DeduplicationStrategy interface {
	ShouldTransfer(offset int64, data []byte) bool
	RecordTransfer(offset int64, data []byte)
	SaveState() error
	LoadState() error
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
		b := &BloomFilterDedup{
			filter:    bloom.NewWithEstimates(1000000, 0.01),
			stateFile: cfg.DedupStateFile,
		}
		if _, err := os.Stat(cfg.DedupStateFile); err == nil {
			b.LoadState()
		}
		return b
	default:
		c := &ChecksumDedup{
			stateFile: cfg.DedupStateFile,
			hashes:    make(map[int64][32]byte),
		}
		if _, err := os.Stat(cfg.DedupStateFile); err == nil {
			c.LoadState()
		}
		return c
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

func (c *ChecksumDedup) LoadState() error {
	file, err := os.Open(c.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hashes == nil {
		c.hashes = make(map[int64][32]byte)
	}

	for {
		var offset int64
		if err := binary.Read(file, binary.LittleEndian, &offset); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		var hash [32]byte
		if _, err := io.ReadFull(file, hash[:]); err != nil {
			return err
		}
		c.hashes[offset] = hash
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
	b.mu.RLock()
	defer b.mu.RUnlock()

	file, err := os.Create(b.stateFile)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = b.filter.WriteTo(file)
	return err
}

func (b *BloomFilterDedup) LoadState() error {
	file, err := os.Open(b.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	b.mu.Lock()
	defer b.mu.Unlock()

	_, err = b.filter.ReadFrom(file)
	return err
}
