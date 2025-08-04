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
	"go.uber.org/zap"
)

var createStateFile = func(name string) (io.WriteCloser, error) {
	return os.Create(name)
}

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
	entries   uint
	fpRate    float64
}

type RollingHashDedup struct {
	stateFile string
	hashes    map[int64]uint64
	mu        sync.RWMutex
}

func NewDeduplicationStrategy(cfg *config.Config) DeduplicationStrategy {
	switch cfg.DedupStrategy {
	case "rolling_hash":
		d := &RollingHashDedup{
			stateFile: cfg.DedupStateFile,
			hashes:    make(map[int64]uint64),
		}
		if err := d.loadState(); err != nil {
			zap.L().Warn("failed to load dedup state", zap.Error(err))
		}
		return d
	case "bloom":
		d := &BloomFilterDedup{
			filter:    bloom.NewWithEstimates(uint(cfg.BloomEntries), cfg.BloomFpRate),
			stateFile: cfg.DedupStateFile,
			entries:   uint(cfg.BloomEntries),
			fpRate:    cfg.BloomFpRate,
		}
		if err := d.loadState(); err != nil {
			zap.L().Warn("failed to load dedup state", zap.Error(err))
		}
		return d
	default:
		d := &ChecksumDedup{
			stateFile: cfg.DedupStateFile,
			hashes:    make(map[int64][32]byte),
		}
		if err := d.loadState(); err != nil {
			zap.L().Warn("failed to load dedup state", zap.Error(err))
		}
		return d
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

	file, err := createStateFile(c.stateFile)
	if err != nil {
		return err
	}
	defer file.Close()

	for offset, hash := range c.hashes {
		if err := binary.Write(file, binary.LittleEndian, offset); err != nil {
			return err
		}
		if _, err := file.Write(hash[:]); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChecksumDedup) loadState() error {
	file, err := os.Open(c.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

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

func (r *RollingHashDedup) computeHash(data []byte) uint64 {
	const base uint64 = 257
	var h uint64
	for _, b := range data {
		h = h*base + uint64(b)
	}
	return h
}

func (r *RollingHashDedup) ShouldTransfer(offset int64, data []byte) bool {
	h := r.computeHash(data)

	r.mu.RLock()
	prev, exists := r.hashes[offset]
	r.mu.RUnlock()

	return !exists || prev != h
}

func (r *RollingHashDedup) RecordTransfer(offset int64, data []byte) {
	h := r.computeHash(data)

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

func (r *RollingHashDedup) loadState() error {
	file, err := os.Open(r.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hashes == nil {
		r.hashes = make(map[int64]uint64)
	}

	for {
		var offset int64
		if err := binary.Read(file, binary.LittleEndian, &offset); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		var hash uint64
		if err := binary.Read(file, binary.LittleEndian, &hash); err != nil {
			return err
		}
		r.hashes[offset] = hash
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

	file, err := os.Create(b.stateFile)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = b.filter.WriteTo(file)
	return err
}

func (b *BloomFilterDedup) loadState() error {
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

	if b.filter == nil {
		b.filter = bloom.NewWithEstimates(b.entries, b.fpRate)
	}

	_, err = b.filter.ReadFrom(file)
	return err
}
