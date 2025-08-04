//go:build amd64 || 386

package transfer

import "github.com/klauspost/cpuid/v2"

func detectOptimalCompression() string {
	if cpuid.CPU.Has(cpuid.AVX512F) || cpuid.CPU.Has(cpuid.AVX2) || cpuid.CPU.Has(cpuid.BMI2) {
		return compressionZSTD
	}
	return compressionLZ4
}
