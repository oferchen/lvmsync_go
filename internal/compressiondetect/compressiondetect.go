package compressiondetect

import (
	"bytes"
	"io"
	"math/rand"
	"sync"
	"time"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/klauspost/cpuid/v2"
	"github.com/pierrec/lz4/v4"
)

var (
	detectOnce sync.Once
	detected   string
)

// DetectOptimalCompression determines the fastest compression algorithm for the current CPU.
func DetectOptimalCompression() string {
	detectOnce.Do(func() {
		if cpuid.CPU.Has(cpuid.AVX512F) || cpuid.CPU.Has(cpuid.AVX2) || cpuid.CPU.Has(cpuid.BMI2) || cpuid.CPU.Has(cpuid.SSE42) || cpuid.CPU.Has(cpuid.ASIMD) || cpuid.CPU.Has(cpuid.SVE) {
			detected = "zstd"
		} else {
			detected = BenchmarkCompression()
		}
	})
	return detected
}

// BenchmarkCompression runs a tiny benchmark using both lz4 and zstd compressors and returns the faster algorithm.
func BenchmarkCompression() string {
	sample := make([]byte, 1<<16) // 64KB sample block
	copy(sample[:1<<15], bytes.Repeat([]byte("a"), 1<<15))
	prng := rand.New(rand.NewSource(0))
	if _, err := prng.Read(sample[1<<15:]); err != nil {
		return "lz4"
	}

	lz4Start := time.Now()
	lw := lz4.NewWriter(io.Discard)
	if _, err := lw.Write(sample); err != nil {
		return "lz4"
	}
	if err := lw.Close(); err != nil {
		return "lz4"
	}
	lz4Dur := time.Since(lz4Start)

	zw, err := zstd.NewWriter(io.Discard)
	if err != nil {
		return "lz4"
	}
	zstdStart := time.Now()
	if _, err := zw.Write(sample); err != nil {
		zw.Close()
		return "lz4"
	}
	if err := zw.Close(); err != nil {
		return "lz4"
	}
	zstdDur := time.Since(zstdStart)

	if zstdDur < lz4Dur {
		return "zstd"
	}
	return "lz4"
}

// ResetForTest clears cached detection results; intended for use in tests.
func ResetForTest() {
	detectOnce = sync.Once{}
	detected = ""
}
