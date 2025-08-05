//go:build arm64

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
		if cpuid.CPU.Has(cpuid.ASIMD) || cpuid.CPU.Has(cpuid.SVE) {
			detected = compressionZSTD
		} else {
			detected = benchmarkCompression()
		}
	})
	return detected
}
