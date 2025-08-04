// transfer/compression.go
package transfer

import (
	"fmt"
	"io"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

const (
	compressionLZ4  = "lz4"
	compressionZSTD = "zstd"
)

// No shared state is kept between decompression readers.

func NewCompressionWriter(dst io.Writer, compress string, level int) (io.WriteCloser, error) {
	if compress == "auto" {
		compress = detectOptimalCompression()
	}

	switch compress {
	case "none":
		return nopWriteCloser{dst}, nil
	case compressionLZ4:
		// For LZ4, valid levels include lz4.Fast and lz4.Level1 through lz4.Level9.
		w := lz4.NewWriter(dst)
		w.Apply(lz4.CompressionLevelOption(lz4.CompressionLevel(level)))
		return w, nil
	case compressionZSTD:
		// Ensure provided level is within the supported zstd range.
		if level < 1 || level > 22 {
			return nil, fmt.Errorf("invalid zstd compression level: %d", level)
		}
		encLevel := zstd.EncoderLevelFromZstd(level)
		return zstd.NewWriter(dst, zstd.WithEncoderLevel(encLevel))
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
