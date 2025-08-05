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

func NewCompressionWriter(dst io.Writer, compress string, level int, concurrency int) (io.WriteCloser, error) {
	if compress == "auto" {
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

func NewDecompressionReader(r io.Reader, compress string) (io.ReadCloser, error) {
	strategy, ok := compressionStrategies[compress]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
	return strategy.NewReader(r)
}
