package transfer

import (
	"fmt"
	"io"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

type CompressionStrategy interface {
	NewWriter(dst io.Writer, level int, concurrency int) (io.WriteCloser, error)
	NewReader(src io.Reader) (io.ReadCloser, error)
}

type noneStrategy struct{}

func (noneStrategy) NewWriter(dst io.Writer, level int, concurrency int) (io.WriteCloser, error) {
	return nopWriteCloser{dst}, nil
}

func (noneStrategy) NewReader(src io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(src), nil
}

type lz4Strategy struct{}

func (lz4Strategy) NewWriter(dst io.Writer, level int, concurrency int) (io.WriteCloser, error) {
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
}

func (lz4Strategy) NewReader(src io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(lz4.NewReader(src)), nil
}

type zstdStrategy struct{}

func (zstdStrategy) NewWriter(dst io.Writer, level int, concurrency int) (io.WriteCloser, error) {
	if level < 1 || level > 22 {
		return nil, fmt.Errorf("invalid zstd compression level: %d", level)
	}
	encLevel := zstd.EncoderLevelFromZstd(level)
	opts := []zstd.EOption{zstd.WithEncoderLevel(encLevel), zstd.WithEncoderConcurrency(concurrency)}
	return zstd.NewWriter(dst, opts...)
}

func (zstdStrategy) NewReader(src io.Reader) (io.ReadCloser, error) {
	decoder, err := zstd.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize zstd decoder: %w", err)
	}
	return &zstdReadCloser{Decoder: decoder}, nil
}

var compressionStrategies = map[string]CompressionStrategy{
	"none":          noneStrategy{},
	compressionLZ4:  lz4Strategy{},
	compressionZSTD: zstdStrategy{},
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
