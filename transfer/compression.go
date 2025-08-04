// transfer/compression.go
package transfer

import (
	"fmt"
	"io"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"golang.org/x/sys/cpu"
	"runtime"
)

const (
	compressionLZ4  = "lz4"
	compressionZSTD = "zstd"
)

// No shared state is kept between decompression readers.

func detectOptimalCompression() string {
	if cpu.X86.HasAVX2 {
		return compressionZSTD
	}
	return compressionLZ4
}

func NewCompressionWriter(w io.Writer, compress string, level int) (io.WriteCloser, error) {
	if compress == "auto" {
		compress = detectOptimalCompression()
	}

	switch compress {
	case "none":
		return nopWriteCloser{w}, nil
	case compressionLZ4:
		return lz4.NewWriter(w), nil
	case compressionZSTD:
		return zstd.NewWriter(w,
			zstd.WithEncoderLevel(zstd.EncoderLevel(level)),
			zstd.WithEncoderConcurrency(runtime.GOMAXPROCS(0)))
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
}

func NewDecompressionReader(r io.Reader, compress string) (io.ReadCloser, error) {
	switch compress {
	case "none":
		return nopReadCloser{r}, nil
	case compressionLZ4:
		return nopReadCloser{lz4.NewReader(r)}, nil
	case compressionZSTD:
		decoder, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(runtime.GOMAXPROCS(0)))
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

type nopReadCloser struct {
	io.Reader
}

func (nopReadCloser) Close() error { return nil }

type zstdReadCloser struct {
	*zstd.Decoder
}

func (z *zstdReadCloser) Close() error {
	z.Decoder.Close()
	return nil
}
