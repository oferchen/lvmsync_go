// Package hash exposes CPU feature detection to allow callers to log chosen
// SIMD paths or make algorithm decisions.
package hash

import "github.com/klauspost/cpuid/v2"

// SIMD reports available SIMD extensions.
type SIMD struct {
	AVX512 bool
	AVX2   bool
	SSE41  bool
}

// Detect returns SIMD capabilities of the current CPU.
func Detect() SIMD {
	cpu := cpuid.CPU
	return SIMD{
		AVX512: cpu.Supports(cpuid.AVX512F),
		AVX2:   cpu.Supports(cpuid.AVX2),
		SSE41:  cpu.Supports(cpuid.SSE4),
	}
}
