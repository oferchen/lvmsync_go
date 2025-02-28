// transfer/compression.go
package transfer

import (
	"fmt"
	"io"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"golang.org/x/sys/cpu"
)

func NewCompressionWriter(w io.Writer, compress string, level int) (io.WriteCloser, error) {
	if compress == "auto" {
		if cpu.X86.HasAVX2 {
			compress = "zstd"
		} else {
			compress = "lz4"
		}
	}
	switch compress {
	case "none":
		return nopWriteCloser{w}, nil
	case "lz4":
		return lz4.NewWriter(w), nil
	case "zstd":
		return zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevel(level)))
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
}

func NewDecompressionReader(r io.Reader, compress string) (io.ReadCloser, error) {
	switch compress {
	case "none":
		return nopReadCloser{r}, nil
	case "lz4":
		return nopReadCloser{lz4.NewReader(r)}, nil
	case "zstd":
		dec, err := zstd.NewReader(r)
		if err != nil {
			return nil, err
		}
		return &zstdReadCloser{Decoder: dec}, nil
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
