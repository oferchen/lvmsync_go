// transfer/compression.go
package transfer

import (
	"fmt"
	"io"
	"runtime"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

const (
	compressionLZ4  = "lz4"
	compressionZSTD = "zstd"
)

// No shared state is kept between decompression readers.

func NewCompressionWriter(dst io.Writer, compress string, level int, concurrency int) (io.WriteCloser, error) {
	if compress == "auto" {
		compress = detectOptimalCompression()
	}

	if concurrency <= 0 {
		concurrency = runtime.GOMAXPROCS(0)
	}

	switch compress {
	case "none":
		return nopWriteCloser{dst}, nil
	case compressionLZ4:
		// Validate level prior to constructing the writer to avoid applying an invalid option.
		lvl := lz4.CompressionLevel(level)
		switch lvl {
		case lz4.Fast, lz4.Level1, lz4.Level2, lz4.Level3, lz4.Level4, lz4.Level5, lz4.Level6, lz4.Level7, lz4.Level8, lz4.Level9:
		// valid
		default:
			return nil, fmt.Errorf("invalid lz4 compression level: %d", level)
		}
		w := lz4.NewWriter(dst)
		if err := w.Apply(lz4.CompressionLevelOption(lvl)); err != nil {
			return nil, fmt.Errorf("failed to apply lz4 compression level: %w", err)
		}
		return w, nil
	case compressionZSTD:
		// Ensure provided level is within the supported zstd range.
		if level < 1 || level > 22 {
			return nil, fmt.Errorf("invalid zstd compression level: %d", level)
		}
		encLevel := zstd.EncoderLevelFromZstd(level)
		opts := []zstd.EOption{zstd.WithEncoderLevel(encLevel), zstd.WithEncoderConcurrency(concurrency)}
		return zstd.NewWriter(dst, opts...)
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
}

func NewDecompressionReader(r io.Reader, compress string) (io.ReadCloser, error) {
	switch compress {
	case "none":
		return io.NopCloser(r), nil
	case compressionLZ4:
		return io.NopCloser(lz4.NewReader(r)), nil
	case compressionZSTD:
		decoder, err := zstd.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize zstd decoder: %w", err)
		}
		return &zstdReadCloser{Decoder: decoder}, nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

type zstdReadCloser struct {
	*zstd.Decoder
}

func (z *zstdReadCloser) Close() error {
	z.Decoder.Close()
	return nil
}
