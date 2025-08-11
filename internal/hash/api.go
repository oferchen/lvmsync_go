// Package hash provides pluggable hashing algorithms used throughout the
// transfer pipeline.
package hash

// Hasher sums a byte slice into a 32-byte digest.
type Hasher interface {
	Sum(b []byte) [32]byte
}
