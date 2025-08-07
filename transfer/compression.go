// transfer/compression.go
package transfer

import (
	"fmt"
	"io"
	"runtime"

	"lvmsync_go/internal/compressiondetect"
)

const (
	compressionLZ4  = "lz4"
	compressionZSTD = "zstd"
)

// No shared state is kept between decompression readers.

// NewCompressionWriter creates a compression writer that wraps the destination
// `dst`. The `compress` parameter selects the compression algorithm and may
// be set to StrategyAuto to automatically detect the optimal compression. The
// `level` parameter controls the compression level when supported. If
// `concurrency` is less than or equal to zero, it defaults to the number of
// logical CPUs. It returns the configured `io.WriteCloser` or an error if the
// compression type is unsupported.
func NewCompressionWriter(dst io.Writer, compress string, level int, concurrency int) (io.WriteCloser, error) {
	if compress == StrategyAuto {
		compress = compressiondetect.DetectOptimalCompression()
	}

	if concurrency <= 0 {
		concurrency = runtime.GOMAXPROCS(0)
	}

	strategy, ok := compressionStrategies[compress]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
	return strategy.NewWriter(dst, level, concurrency)
}

// NewDecompressionReader returns a reader that decompresses data from `r`
// using the specified `compress` strategy. If `concurrency` is less than or
// equal to zero, the number of logical CPUs is used. An `io.ReadCloser` is
// returned to provide the decompressed stream, or an error if the compression
// type is unsupported.
func NewDecompressionReader(r io.Reader, compress string, concurrency int) (io.ReadCloser, error) {
	if concurrency <= 0 {
		concurrency = runtime.GOMAXPROCS(0)
	}
	strategy, ok := compressionStrategies[compress]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
	return strategy.NewReader(r, concurrency)
}
