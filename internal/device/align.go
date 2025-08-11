// Package device includes alignment helpers for block operations.
package device

// AlignDown returns x rounded down to the nearest multiple of a.
func AlignDown(x, a uint64) uint64 { return x & ^(a - 1) }

// AlignUp returns x rounded up to the nearest multiple of a.
func AlignUp(x, a uint64) uint64 { return (x + a - 1) & ^(a - 1) }
