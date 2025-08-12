package cpufeatures

import "golang.org/x/sys/cpu"

// HasAESNI reports whether AES-NI is supported on the current CPU.
func HasAESNI() bool {
	return cpu.X86.HasAES
}

// HasAVX2 reports AVX2 support on x86 CPUs.
func HasAVX2() bool {
	return cpu.X86.HasAVX2
}

// HasAVX512 reports AVX-512 support on x86 CPUs.
func HasAVX512() bool {
	return cpu.X86.HasAVX512F
}

// HasNEON reports whether the CPU has NEON/ASIMD support.
func HasNEON() bool {
	return cpu.ARM64.HasASIMD || cpu.ARM.HasNEON
}

// HasSIMD reports if any major SIMD extension is present.
func HasSIMD() bool {
	return HasAVX512() || HasAVX2() || HasNEON()
}
