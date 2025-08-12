// transfer/compression.go
package transfer

import (
	"bytes"
	"fmt"
	"io"
	"runtime"

	"github.com/pierrec/lz4/v4"

	"lvmsync_go/internal/compressiondetect"
	cpufeatures "lvmsync_go/internal/cpufeatures"
)

const (
	compressionLZ4  = "lz4"
	compressionZSTD = "zstd"
)

// supportsSIMD reports whether the CPU has SIMD acceleration. It is
// a variable to allow tests to override detection behavior.
var supportsSIMD = cpufeatures.HasSIMD

// hasNEON reports whether the current CPU supports NEON instructions. It is
// a variable to allow tests to override the detection behavior.
var hasNEON = compressiondetect.HasNEON

// No shared state is kept between decompression readers.

// NewCompressionWriter creates a compression writer that wraps the destination
// `dst`. The `compress` parameter selects the compression algorithm and may
// be set to StrategyAuto to automatically detect the optimal compression. The
// `level` parameter controls the compression level when supported. If
// `concurrency` is less than or equal to zero, it defaults to the number of
// logical CPUs. It returns the configured `io.WriteCloser` or an error if the
// compression type is unsupported.
func NewCompressionWriter(dst io.Writer, compress string, level, concurrency int) (io.WriteCloser, error) {
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

const (
	sampleSize    = 8 * 1024
	lz4MaxChunk   = 256 * 1024
	defaultZstdLv = 1
	maxAutoZstdLv = 3
)

// selectAlgorithm chooses a compression algorithm and level based on the
// requested strategy, chunk size, and CPU capabilities.
func selectAlgorithm(chunkLen int, compress string, level int) (string, int) {
	if compress == StrategyAuto {
		if chunkLen < lz4MaxChunk {
			if level == 0 {
				level = int(lz4.Level1)
			}
			return compressionLZ4, level
		}
		if supportsSIMD() {
			return compressionZSTD, defaultZstdLv
		if hasAVX2() || hasNEON() {
			if level <= 0 {
				level = defaultZstdLv
			} else if level > maxAutoZstdLv {
				level = maxAutoZstdLv
			}
			return compressionZSTD, level
		}
		if level == 0 {
			level = int(lz4.Level1)
		}
		return compressionLZ4, level
	}
	return compress, level
}

// estimateRatio compresses a sample of the data using the selected algorithm
// and returns the compressed size ratio.
func estimateRatio(data []byte, algo string, level, concurrency int) (float64, error) {
	n := len(data)
	if n > sampleSize {
		n = sampleSize
	}
	sample := data[:n]
	var buf bytes.Buffer
	w, err := NewCompressionWriter(&buf, algo, level, concurrency)
	if err != nil {
		return 0, err
	}
	if _, err := w.Write(sample); err != nil {
		_ = w.Close()
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	return float64(buf.Len()) / float64(n), nil
}

// CompressChunk compresses a chunk of data based on the provided compression
// settings. Compression is skipped when the estimated ratio exceeds the
// threshold. The returned string indicates the algorithm used; "none" means no
// compression was applied.
func CompressChunk(data []byte, compress string, level, concurrency int, threshold float64) ([]byte, string, error) {
	algo, lvl := selectAlgorithm(len(data), compress, level)
	if algo == "none" {
		return data, "none", nil
	}
	ratio, err := estimateRatio(data, algo, lvl, concurrency)
	if err != nil {
		return nil, "", err
	}
	if ratio >= threshold {
		return data, "none", nil
	}
	var buf bytes.Buffer
	w, err := NewCompressionWriter(&buf, algo, lvl, concurrency)
	if err != nil {
		return nil, "", err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), algo, nil
}
