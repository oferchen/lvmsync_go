//go:build arm64

package transfer

import "github.com/klauspost/cpuid/v2"

func detectOptimalCompression() string {
	if cpuid.CPU.Has(cpuid.ASIMD) {
		return compressionZSTD
	}
	return compressionLZ4
}
