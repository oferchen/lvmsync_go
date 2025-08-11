// Package hash exposes a BLAKE3 implementation leveraging the zeebo/blake3
// library, which performs SIMD auto-detection internally.
package hash

import "github.com/zeebo/blake3"

// Blake3 returns a Hasher using the BLAKE3 algorithm.
type Blake3 struct{}

// NewBlake3 creates a new BLAKE3 hasher.
func NewBlake3() Hasher { return Blake3{} }

// Sum computes a 32-byte digest of the provided data.
func (Blake3) Sum(b []byte) [32]byte { return blake3.Sum256(b) }
