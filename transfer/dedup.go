// transfer/dedup.go
package transfer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"io"
	"math"
	"os"
	"sync"
	"unsafe"

	"github.com/bits-and-blooms/bloom/v3"
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
	cpufeatures "github.com/oferchen/lvmsync_go/internal/cpufeatures"
)

// saveStateFile writes dedup state to the provided path; logger may be nil.
func saveStateFile(deps *Deps, logger *zap.Logger, path string, write func(io.Writer) error) error {
	file, err := deps.CreateStateFile(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			if logger != nil {
				logger.Warn("failed to close state file", zap.Error(closeErr))
			}
		}
	}()
	return write(file)
}

// DeduplicationStrategy defines a block deduplication algorithm.
// Implementations decide if a block should be transferred and persist their state.
type DeduplicationStrategy interface {
	ShouldTransfer(offset int64, data []byte) bool
	RecordTransfer(offset int64, data []byte)
	SaveState() error
}

// ChecksumDedup stores block checksums for deduplication.
// It persists hash data to a state file on disk.
type ChecksumDedup struct {
	stateFile string
	hashes    map[int64][]byte
	mu        sync.RWMutex
	strategy  ChecksumStrategy
	logger    *zap.Logger
	deps      *Deps
}

// BloomFilterDedup uses a Bloom filter to track transferred blocks.
// It balances memory usage and false-positive rate.
type BloomFilterDedup struct {
	filter    *bloom.BloomFilter
	stateFile string
	mu        sync.RWMutex
	entries   uint
	fpRate    float64
	strategy  ChecksumStrategy
	logger    *zap.Logger
	deps      *Deps
	lookups   uint64
	misses    uint64
}

// RollingHashDedup computes a rolling hash for block comparison.
// It is optimized for CPUs without SIMD support.
type RollingHashDedup struct {
	stateFile string
	hashes    map[int64]uint64
	mu        sync.RWMutex
	seed      maphash.Seed
	logger    *zap.Logger
	deps      *Deps
}

var rollingHashPool = sync.Pool{
	New: func() any { return new(maphash.Hash) },
}

func supportsChecksumAcceleration() bool {
	return cpufeatures.HasAVX2() || cpufeatures.HasAVX512() || cpufeatures.HasNEON() || cpufeatures.HasAESNI()
}

// NewDeduplicationStrategy returns a deduplication strategy based on cfg.
// When cfg.DedupStrategy is auto, it selects the best available algorithm
// and updates cfg accordingly.
func NewDeduplicationStrategy(cfg *config.Config, logger *zap.Logger) DeduplicationStrategy {
	return NewDeduplicationStrategyWithDeps(cfg, logger, DefaultDeps)
}

func NewDeduplicationStrategyWithDeps(cfg *config.Config, logger *zap.Logger, deps *Deps) DeduplicationStrategy {
	strategy := cfg.DedupStrategy
	if strategy == StrategyAuto {
		strategy = deps.DetectBestStrategy()
		cfg.DedupStrategy = strategy
	}
	switch strategy {
	case "none":
		return nil
	case StrategyRollingHash:
		d := &RollingHashDedup{
			stateFile: cfg.DedupStateFile,
			hashes:    make(map[int64]uint64),
			seed:      maphash.MakeSeed(),
			logger:    logger,
			deps:      deps,
		}
		if err := d.loadState(); err != nil {
			logger.Warn("failed to load dedup state", zap.Error(err))
		}
		return d
	case "bloom":
		if cfg.BloomEntries < 0 || uint64(cfg.BloomEntries) > uint64(math.MaxUint) {
			logger.Warn("invalid bloom entries", zap.Int("entries", cfg.BloomEntries))
			return nil
		}
		entries := uint(cfg.BloomEntries)
		d := &BloomFilterDedup{
			filter:    bloom.NewWithEstimates(entries, cfg.BloomFpRate),
			stateFile: cfg.DedupStateFile,
			entries:   entries,
			fpRate:    cfg.BloomFpRate,
			strategy:  GetChecksumStrategy(cfg.ChecksumAlgorithm),
			logger:    logger,
			deps:      deps,
		}
		if err := d.loadState(); err != nil {
			logger.Warn("failed to load dedup state", zap.Error(err))
		}
		return d
	default:
		d := &ChecksumDedup{
			stateFile: cfg.DedupStateFile,
			hashes:    make(map[int64][]byte),
			strategy:  GetChecksumStrategy(cfg.ChecksumAlgorithm),
			logger:    logger,
			deps:      deps,
		}
		if err := d.loadState(); err != nil {
			logger.Warn("failed to load dedup state", zap.Error(err))
		}
		return d
	}
}

func (c *ChecksumDedup) ShouldTransfer(offset int64, data []byte) bool {
	c.mu.RLock()
	prev, exists := c.hashes[offset]
	c.mu.RUnlock()

	sum := c.strategy.Compute(data)
	return !exists || !bytes.Equal(prev, sum)
}

func (c *ChecksumDedup) RecordTransfer(offset int64, data []byte) {
	sum := c.strategy.Compute(data)

	c.mu.Lock()
	c.hashes[offset] = sum
	c.mu.Unlock()
}

func (c *ChecksumDedup) SaveState() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return saveStateFile(c.deps, c.logger, c.stateFile, func(file io.Writer) error {
		size := c.strategy.Size()
		for offset, hash := range c.hashes {
			if err := binary.Write(file, binary.LittleEndian, offset); err != nil {
				return err
			}
			if len(hash) != size {
				continue
			}
			if _, err := file.Write(hash); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *ChecksumDedup) loadState() error {
	file, err := os.Open(c.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			c.logger.Warn("failed to close state file", zap.Error(closeErr))
		}
	}()

	size := c.strategy.Size()
	for {
		var offset int64
		if err := binary.Read(file, binary.LittleEndian, &offset); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		hash := make([]byte, size)
		if _, err := io.ReadFull(file, hash); err != nil {
			return err
		}
		c.hashes[offset] = hash
	}
	return nil
}

func (r *RollingHashDedup) computeHash(data []byte) (uint64, error) {
	hAny := rollingHashPool.Get()
	h, ok := hAny.(*maphash.Hash)
	if !ok {
		rollingHashPool.Put(hAny)
		return 0, fmt.Errorf("unexpected type %T from rollingHashPool", hAny)
	}
	h.Reset()
	h.SetSeed(r.seed)
	h.Write(data)
	sum := h.Sum64()
	rollingHashPool.Put(h)
	return sum, nil
}

func (r *RollingHashDedup) ShouldTransfer(offset int64, data []byte) bool {
	h, err := r.computeHash(data)
	if err != nil {
		r.logger.Error("compute hash failed", zap.Error(err))
		return true
	}

	r.mu.RLock()
	prev, exists := r.hashes[offset]
	r.mu.RUnlock()

	return !exists || prev != h
}

func (r *RollingHashDedup) RecordTransfer(offset int64, data []byte) {
	h, err := r.computeHash(data)
	if err != nil {
		r.logger.Error("compute hash failed", zap.Error(err))
		return
	}

	r.mu.Lock()
	r.hashes[offset] = h
	r.mu.Unlock()
}

func (r *RollingHashDedup) SaveState() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return saveStateFile(r.deps, r.logger, r.stateFile, func(file io.Writer) error {
		seedArr := *(*[2]uint64)(unsafe.Pointer(&r.seed))
		if err := binary.Write(file, binary.LittleEndian, seedArr); err != nil {
			return err
		}
		for offset, hash := range r.hashes {
			if err := binary.Write(file, binary.LittleEndian, offset); err != nil {
				return err
			}
			if err := binary.Write(file, binary.LittleEndian, hash); err != nil {
				return err
			}
		}
		return nil
	})
}

//nolint:revive // complex state loading
func (r *RollingHashDedup) loadState() error {
	file, err := os.Open(r.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			r.logger.Warn("failed to close state file", zap.Error(err))
		}
	}()

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hashes == nil {
		r.hashes = make(map[int64]uint64)
	}

	var seedArr [2]uint64
	if err := binary.Read(file, binary.LittleEndian, &seedArr); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	r.seed = *(*maphash.Seed)(unsafe.Pointer(&seedArr))

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

func (b *BloomFilterDedup) ShouldTransfer(_ int64, data []byte) bool {
	// offset is ignored because the Bloom filter only tracks data hashes.
	sum := b.strategy.Compute(data)
	b.mu.Lock()
	b.lookups++
	ok := !b.filter.Test(sum)
	if ok {
		b.misses++
	}
	b.mu.Unlock()
	return ok
}

func (b *BloomFilterDedup) RecordTransfer(_ int64, data []byte) {
	// offset is ignored because the Bloom filter only tracks data hashes.
	b.mu.Lock()
	b.filter.Add(b.strategy.Compute(data))
	b.mu.Unlock()
}

func (b *BloomFilterDedup) SaveState() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.logger != nil {
		var observed float64
		if b.lookups > 0 {
			observed = float64(b.lookups-b.misses) / float64(b.lookups)
		}
		b.logger.Info("dedup_bloom_stats",
			zap.Uint("entries", uint(b.filter.ApproximatedSize())),
			zap.Float64("configured_fp_rate", b.fpRate),
			zap.Float64("observed_fp_rate", observed),
		)
	}
	return saveStateFile(b.deps, b.logger, b.stateFile, func(file io.Writer) error {
		_, err := b.filter.WriteTo(file)
		return err
	})
}

func (b *BloomFilterDedup) loadState() error {
	file, err := os.Open(b.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			b.logger.Warn("failed to close state file", zap.Error(closeErr))
		}
	}()

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.filter == nil {
		b.filter = bloom.NewWithEstimates(b.entries, b.fpRate)
	}

	_, err = b.filter.ReadFrom(file)
	return err
}
