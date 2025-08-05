package transfer

import (
	"bytes"
	"io"
	"math/rand"
	"time"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

// benchmarkCompression runs a tiny benchmark using both lz4 and zstd
// compressors and returns the algorithm that finishes faster. The
// benchmark is intentionally small so it can run during start-up without
// adding noticeable overhead.
func benchmarkCompression() string {
	sample := make([]byte, 1<<16) // 64KB sample block
	copy(sample[:1<<15], bytes.Repeat([]byte("a"), 1<<15))
	prng := rand.New(rand.NewSource(0))
	if _, err := prng.Read(sample[1<<15:]); err != nil {
		return compressionLZ4
	}

	lz4Start := time.Now()
	lw := lz4.NewWriter(io.Discard)
	if _, err := lw.Write(sample); err != nil {
		return compressionLZ4
	}
	if err := lw.Close(); err != nil {
		return compressionLZ4
	}
	lz4Dur := time.Since(lz4Start)

	zw, err := zstd.NewWriter(io.Discard)
	if err != nil {
		return compressionLZ4
	}
	zstdStart := time.Now()
	if _, err := zw.Write(sample); err != nil {
		zw.Close()
		return compressionLZ4
	}
	if err := zw.Close(); err != nil {
		return compressionLZ4
	}
	zstdDur := time.Since(zstdStart)

	if zstdDur < lz4Dur {
		return compressionZSTD
	}
	return compressionLZ4
}
