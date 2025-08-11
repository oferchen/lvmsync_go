// Package hash also offers a SHA-256 implementation as a portability fallback.
package hash

import "crypto/sha256"

// SHA256 provides the SHA-256 hashing algorithm.
type SHA256 struct{}

// NewSHA256 creates a new SHA-256 hasher.
func NewSHA256() Hasher { return SHA256{} }

// Sum computes the SHA-256 digest of b.
func (SHA256) Sum(b []byte) [32]byte { return sha256.Sum256(b) }
