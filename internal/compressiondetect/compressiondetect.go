package compressiondetect

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/klauspost/cpuid/v2"
	"github.com/pierrec/lz4/v4"
)

var (
	detectOnce sync.Once
	detected   string

	benchMu    sync.Mutex
	benchCache = make(map[string]string)
)

// DetectOptimalCompression determines the fastest compression algorithm for the current CPU.
func DetectOptimalCompression() string {
	detectOnce.Do(func() {
		cores := cpuid.CPU.PhysicalCores
		if cores == 0 {
			cores = runtime.NumCPU()
		}
		cacheSize := 0
		if cpuid.CPU.Cache.L3 > 0 {
			cacheSize += cpuid.CPU.Cache.L3
		}
		if cpuid.CPU.Cache.L2 > 0 {
			cacheSize += cpuid.CPU.Cache.L2
		}

		if cpuid.CPU.Has(cpuid.AVX512F) || cpuid.CPU.Has(cpuid.AVX2) || cpuid.CPU.Has(cpuid.BMI2) || cpuid.CPU.Has(cpuid.SSE42) || (cores >= 4 && cacheSize >= 2<<20) {
			detected = "zstd"
		} else {
			detected = benchmarkCached(cores, cacheSize)
		}
	})
	return detected
}

// BenchmarkCompression runs a tiny benchmark using both lz4 and zstd compressors and returns the faster algorithm.
func BenchmarkCompression() string {
	sample := make([]byte, 1<<16) // 64KB sample block
	copy(sample[:1<<15], bytes.Repeat([]byte("a"), 1<<15))
	if _, err := rand.Read(sample[1<<15:]); err != nil {
		return "lz4"
	}

	lz4Dur := benchLZ4(sample)
	zstdDur := benchZSTD(sample)

	if zstdDur < lz4Dur {
		return "zstd"
	}
	return "lz4"
}

func benchLZ4(sample []byte) time.Duration {
	var buf bytes.Buffer

	start := time.Now()
	lw := lz4.NewWriter(&buf)
	if _, err := lw.Write(sample); err != nil {
		return time.Hour
	}
	if err := lw.Close(); err != nil {
		return time.Hour
	}
	compDur := time.Since(start)

	r := lz4.NewReader(bytes.NewReader(buf.Bytes()))
	decStart := time.Now()
	if _, err := io.Copy(io.Discard, r); err != nil {
		return time.Hour
	}
	decDur := time.Since(decStart)

	return compDur + decDur
}

func benchZSTD(sample []byte) time.Duration {
	var buf bytes.Buffer

	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		return time.Hour
	}
	start := time.Now()
	if _, err = zw.Write(sample); err != nil {
		if closeErr := zw.Close(); closeErr != nil {
			return time.Hour
		}
		return time.Hour
	}
	if err = zw.Close(); err != nil {
		return time.Hour
	}
	compDur := time.Since(start)

	var zr *zstd.Decoder
	zr, err = zstd.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return time.Hour
	}
	decStart := time.Now()
	if _, err = io.Copy(io.Discard, zr); err != nil {
		zr.Close()
		return time.Hour
	}
	zr.Close()
	decDur := time.Since(decStart)

	return compDur + decDur
}

func benchmarkCached(cores, cache int) string {
	key := fmt.Sprintf("%d-%d", cores, cache)
	benchMu.Lock()
	if res, ok := benchCache[key]; ok {
		benchMu.Unlock()
		return res
	}
	benchMu.Unlock()

	res := BenchmarkCompression()
	benchMu.Lock()
	benchCache[key] = res
	benchMu.Unlock()
	return res
}

// ResetForTest clears cached detection results; intended for use in tests.
func ResetForTest() {
	detectOnce = sync.Once{}
	detected = ""
	benchMu.Lock()
	benchCache = make(map[string]string)
	benchMu.Unlock()
}
