//go:build amd64 || 386

package transfer

import (
	"sync"

	"github.com/klauspost/cpuid/v2"
)

var (
	detectOnce sync.Once
	detected   string
)

func detectOptimalCompression() string {
	detectOnce.Do(func() {
		if cpuid.CPU.Has(cpuid.AVX512F) || cpuid.CPU.Has(cpuid.AVX2) || cpuid.CPU.Has(cpuid.BMI2) || cpuid.CPU.Has(cpuid.SSE42) {
			detected = compressionZSTD
		} else {
			detected = benchmarkCompression()
		}
	})
	return detected
}
