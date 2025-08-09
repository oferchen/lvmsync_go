package dedup

import (
	"math"

	"github.com/bits-and-blooms/bloom/v3"
)

// Bloom is a thin wrapper around a standard Bloom filter that exposes
// configuration based on a desired false positive rate and number of
// entries. The caller supplies already hashed chunk digests which are
// inserted into the filter.
type Bloom struct {
	filter *bloom.BloomFilter
}

// NewBloom creates a Bloom filter that can hold n entries with the given
// false positive rate. Memory usage is derived from these parameters.
func NewBloom(n uint, fpRate float64) *Bloom {
	m, k := bloom.EstimateParameters(n, fpRate)
	f := bloom.New(m, k)
	return &Bloom{filter: f}
}

// TestAndAdd returns true if the given byte slice is probably in the set.
// The data is hashed and the Bloom filter is updated with the resulting
// digest.
func (b *Bloom) TestAndAdd(digest []byte) bool {
	if b.filter.Test(digest) {
		return true
	}
	b.filter.Add(digest)
	return false
}

// MaxChunks calculates the maximum number of unique chunks that can be
// stored in RAM using a Bloom filter with the specified false positive
// rate. The calculation follows the formula given in the PRD:
//
//	maxChunks = (RAM * 0.4809) / ln(1/falsePositiveRate)
func MaxChunks(ramBytes uint64, fpRate float64) uint64 {
	return uint64((float64(ramBytes) * 0.4809) / math.Log(1.0/fpRate))
}

// AdaptiveAvgChunk computes the average chunk size based on volume size,
// available RAM and false positive rate. The size is clamped between the
// provided minimum and maximum values.
func AdaptiveAvgChunk(volumeSize, ramBytes uint64, fpRate float64, minSize, maxSize uint64) (avg, maxChunks uint64) {
	maxChunks = MaxChunks(ramBytes, fpRate)
	if maxChunks == 0 {
		maxChunks = 1
	}
	avg = volumeSize / maxChunks
	if avg < minSize {
		avg = minSize
	}
	if avg > maxSize {
		avg = maxSize
	}
	return
}
