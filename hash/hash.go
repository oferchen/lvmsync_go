package hash

import (
	"hash"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"

	cpufeatures "lvmsync_go/internal/cpufeatures"
)

var hasSIMD bool

func init() {
	hasSIMD = cpufeatures.HasSIMD()
}

// HasSIMD reports whether CPU SIMD features are available.
func HasSIMD() bool { return hasSIMD }

// SumXXH3 returns the 64-bit XXH3 hash of b.
func SumXXH3(b []byte) uint64 {
	return xxh3.Hash(b)
}

// SumBLAKE3 returns the 256-bit BLAKE3 digest of b.
func SumBLAKE3(b []byte) [32]byte {
	return blake3.Sum256(b)
}

// Blake3Hasher wraps a BLAKE3 hasher with optional keyed mode.
type Blake3Hasher struct {
	h *blake3.Hasher
}

// NewBlake3Hasher constructs a BLAKE3 hasher. If key is nil an unkeyed hasher is used.
func NewBlake3Hasher(key []byte) (*Blake3Hasher, error) {
	var h *blake3.Hasher
	var err error
	if key != nil {
		var k [32]byte
		copy(k[:], key)
		h, err = blake3.NewKeyed(k[:])
		if err != nil {
			return nil, err
		}
	} else {
		h = blake3.New()
	}
	return &Blake3Hasher{h: h}, nil
}

// Reset resets the underlying hasher.
func (h *Blake3Hasher) Reset() { h.h.Reset() }

// Write implements io.Writer for the hasher.
func (h *Blake3Hasher) Write(p []byte) (int, error) { return h.h.Write(p) }

// Sum256 returns the digest and resets the hasher.
func (h *Blake3Hasher) Sum256() [32]byte {
	var out [32]byte
	copy(out[:], h.h.Sum(nil))
	h.h.Reset()
	return out
}

// Digest exposes the underlying hash.Hash.
func (h *Blake3Hasher) Digest() hash.Hash { return h.h }
