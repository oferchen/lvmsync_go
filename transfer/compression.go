// transfer/compression.go
package transfer

import (
	"bytes"
	"fmt"
	"io"
	"runtime"

	"github.com/pierrec/lz4/v4"
	"go.uber.org/zap"

	"lvmsync_go/internal/compressiondetect"
	cpufeatures "lvmsync_go/internal/cpufeatures"
)

const (
	compressionLZ4  = "lz4"
	compressionZSTD = "zstd"
)

// hasNEON reports whether the current CPU supports NEON instructions. It is
// a variable to allow tests to override the detection behavior.
var hasNEON = cpufeatures.HasNEON

// hasAVX2 reports whether the current CPU supports AVX2 instructions. It is
// a variable to allow tests to override the detection behavior.
var hasAVX2 = cpufeatures.HasAVX2

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
// requested strategy, chunk size, and CPU capabilities. Separate level values
// are accepted for LZ4 and Zstd to allow independent tuning via CLI flags.
func selectAlgorithm(chunkLen int, compress string, lz4Level, zstdLevel int) (string, int) {
	// Explicit selection uses the provided algorithm-specific level.
	if compress != StrategyAuto {
		switch compress {
		case compressionLZ4:
			if lz4Level == 0 {
				lz4Level = int(lz4.Level1)
			}
			return compressionLZ4, lz4Level
		case compressionZSTD:
			if zstdLevel == 0 {
				zstdLevel = defaultZstdLv
			}
			return compressionZSTD, zstdLevel
		default:
			return compress, lz4Level
		}
	}
	// Automatic selection: small chunks favour LZ4.
	if chunkLen < lz4MaxChunk {
		if lz4Level == 0 {
			lz4Level = int(lz4.Level1)
		}
		return compressionLZ4, lz4Level
	}
	// Larger chunks choose Zstd when SIMD support is available.
	if hasAVX2() || hasNEON() {
		if zstdLevel <= 0 {
			zstdLevel = defaultZstdLv
		} else if zstdLevel > maxAutoZstdLv {
			zstdLevel = maxAutoZstdLv
		}
		return compressionZSTD, zstdLevel
	}
	if lz4Level == 0 {
		lz4Level = int(lz4.Level1)
	}
	return compressionLZ4, lz4Level
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
// compression was applied. Decisions are logged using the provided logger.
// logger must be non-nil; use zap.NewNop() to disable logging.
func CompressChunk(data []byte, compress string, lz4Level, zstdLevel, concurrency int, threshold float64, logger *zap.Logger) ([]byte, string, error) {
	algo, lvl := selectAlgorithm(len(data), compress, lz4Level, zstdLevel)
	if algo == "none" {
		logger.Debug("compression_disabled")
		return data, "none", nil
	}
	ratio, err := estimateRatio(data, algo, lvl, concurrency)
	if err != nil {
		logger.Debug("compression_ratio_error", zap.Error(err))
		return nil, "", err
	}
	logger.Debug("compression_decision",
		zap.String("algorithm", algo),
		zap.Float64("ratio", ratio),
		zap.Float64("threshold", threshold),
		zap.Int("size_bytes", len(data)),
	)
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
