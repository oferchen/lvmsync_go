package transfer

import (
	"bytes"
	"io"
	"time"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

// benchmarkCompression runs a tiny benchmark using both lz4 and zstd
// compressors and returns the algorithm that finishes faster. The
// benchmark is intentionally small so it can run during start-up without
// adding noticeable overhead.
func benchmarkCompression() string {
	sample := bytes.Repeat([]byte("a"), 1<<16) // 64KB sample block

	lz4Start := time.Now()
	lw := lz4.NewWriter(io.Discard)
	_, _ = lw.Write(sample)
	_ = lw.Close()
	lz4Dur := time.Since(lz4Start)

	zw, err := zstd.NewWriter(io.Discard)
	if err != nil {
		return compressionLZ4
	}
	zstdStart := time.Now()
	_, _ = zw.Write(sample)
	_ = zw.Close()
	zstdDur := time.Since(zstdStart)

	if zstdDur < lz4Dur {
		return compressionZSTD
	}
	return compressionLZ4
}
