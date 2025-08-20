package manifest

import (
	"encoding/binary"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
)

func TestBloomFilterFalsePositiveRate(t *testing.T) {
	const (
		size      = 64 << 20 // 64 MiB device
		blockSize = 4096
	)

	dir := t.TempDir()
	path := filepath.Join(dir, "bloom.man")
	idx, err := Create(path, "dev", size, 0, 0, 0, blockSize, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer idx.Close()

	data := make([]byte, blockSize)
	// Insert a handful of chunks per bloomRange to keep the filter representative.
	for r := uint64(0); r < size; r += bloomRange {
		for j := 0; j < 3; j++ {
			off := r + uint64(j)*blockSize
			binary.LittleEndian.PutUint64(data, off)
			xx := xxh3.Hash(data)
			dig := blake3.Sum256(data)
			if err := idx.Set(off, blockSize, 0, xx, dig); err != nil {
				t.Fatalf("Set: %v", err)
			}
		}
	}

	expectedRanges := int((size + bloomRange - 1) / bloomRange)
	if len(idx.bloom) != expectedRanges {
		t.Fatalf("bloom size: want %d ranges, got %d", expectedRanges, len(idx.bloom))
	}

	const trials = 100000
	rng := rand.New(rand.NewSource(1))
	falsePos := 0
	for i := 0; i < trials; i++ {
		off := uint64(rng.Int63n(int64(size)))
		r := off / bloomRange
		h := rng.Uint64()
		bit1 := h & 63
		bit2 := (h >> 6) & 63
		mask := (uint64(1) << bit1) | (uint64(1) << bit2)
		if idx.bloom[r]&mask == mask {
			falsePos++
		}
	}

	rate := float64(falsePos) / trials
	if rate > 0.01 {
		t.Fatalf("false positive rate %.4f exceeds 1%%", rate)
	}
}
