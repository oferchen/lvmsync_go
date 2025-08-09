package transfer

import (
	"fmt"
	"io"
	"sync"

	"github.com/pierrec/lz4/v4"

	zstd "github.com/klauspost/compress/zstd"
)

type CompressionStrategy interface {
	NewWriter(dst io.Writer, level, concurrency int) (io.WriteCloser, error)
	NewReader(src io.Reader, concurrency int) (io.ReadCloser, error)
}

type noneStrategy struct{}

func (noneStrategy) NewWriter(dst io.Writer, _, _ int) (io.WriteCloser, error) {
	// level and concurrency are ignored for the none strategy.
	return nopWriteCloser{dst}, nil
}

func (noneStrategy) NewReader(src io.Reader, _ int) (io.ReadCloser, error) {
	// concurrency is ignored for the none strategy.
	return io.NopCloser(src), nil
}

type lz4Strategy struct{}

var (
	lz4WriterPool sync.Pool
	lz4ReaderPool sync.Pool
)

type pooledLz4WriterEntry struct {
	w           *lz4.Writer
	level       int
	concurrency int
}

type pooledLz4Writer struct {
	*lz4.Writer
	entry *pooledLz4WriterEntry
}

func (w *pooledLz4Writer) Close() error {
	err := w.Writer.Close()
	w.Reset(nil)
	lz4WriterPool.Put(w.entry)
	return err
}

func (lz4Strategy) NewWriter(dst io.Writer, level, concurrency int) (io.WriteCloser, error) {
	var lvl lz4.CompressionLevel
	switch level {
	case int(lz4.Fast), int(lz4.Level1), int(lz4.Level2), int(lz4.Level3), int(lz4.Level4), int(lz4.Level5), int(lz4.Level6), int(lz4.Level7), int(lz4.Level8), int(lz4.Level9):
		lvl = lz4.CompressionLevel(level)
	default:
		return nil, fmt.Errorf("invalid lz4 compression level: %d", level)
	}

	entryAny := lz4WriterPool.Get()
	var entry *pooledLz4WriterEntry
	if entryAny == nil {
		entry = &pooledLz4WriterEntry{}
	} else {
		var ok bool
		entry, ok = entryAny.(*pooledLz4WriterEntry)
		if !ok {
			return nil, fmt.Errorf("invalid lz4 writer entry type")
		}
	}

	if entry.w == nil {
		entry.w = lz4.NewWriter(dst)
		if err := entry.w.Apply(lz4.CompressionLevelOption(lvl), lz4.ConcurrencyOption(concurrency)); err != nil {
			lz4WriterPool.Put(entry)
			return nil, fmt.Errorf("failed to apply lz4 options: %w", err)
		}
		entry.level = level
		entry.concurrency = concurrency
	} else {
		entry.w.Reset(dst)
		if entry.level != level || entry.concurrency != concurrency {
			if err := entry.w.Apply(lz4.CompressionLevelOption(lvl), lz4.ConcurrencyOption(concurrency)); err != nil {
				lz4WriterPool.Put(entry)
				return nil, fmt.Errorf("failed to apply lz4 options: %w", err)
			}
			entry.level = level
			entry.concurrency = concurrency
		}
	}

	return &pooledLz4Writer{Writer: entry.w, entry: entry}, nil
}

type pooledLz4ReaderEntry struct {
	r           *lz4.Reader
	concurrency int
}

type pooledLz4Reader struct {
	*lz4.Reader
	entry *pooledLz4ReaderEntry
}

func (r *pooledLz4Reader) Close() error {
	r.Reset(nil)
	lz4ReaderPool.Put(r.entry)
	return nil
}

func (lz4Strategy) NewReader(src io.Reader, concurrency int) (io.ReadCloser, error) {
	entryAny := lz4ReaderPool.Get()
	var entry *pooledLz4ReaderEntry
	if entryAny == nil {
		entry = &pooledLz4ReaderEntry{}
	} else {
		var ok bool
		entry, ok = entryAny.(*pooledLz4ReaderEntry)
		if !ok {
			return nil, fmt.Errorf("invalid lz4 reader entry type")
		}
	}

	if entry.r == nil {
		entry.r = lz4.NewReader(src)
		if err := entry.r.Apply(lz4.ConcurrencyOption(concurrency)); err != nil {
			lz4ReaderPool.Put(entry)
			return nil, fmt.Errorf("failed to apply lz4 options: %w", err)
		}
		entry.concurrency = concurrency
	} else {
		entry.r.Reset(src)
		if entry.concurrency != concurrency {
			if err := entry.r.Apply(lz4.ConcurrencyOption(concurrency)); err != nil {
				lz4ReaderPool.Put(entry)
				return nil, fmt.Errorf("failed to apply lz4 options: %w", err)
			}
			entry.concurrency = concurrency
		}
	}

	return &pooledLz4Reader{Reader: entry.r, entry: entry}, nil
}

type zstdStrategy struct{}

var (
	zstdEncoderPool sync.Pool
	zstdDecoderPool sync.Pool
)

type pooledZstdEncoder struct {
	enc         *zstd.Encoder
	level       int
	concurrency int
}

type pooledZstdWriter struct {
	*zstd.Encoder
	entry *pooledZstdEncoder
}

func (w *pooledZstdWriter) Close() error {
	err := w.Encoder.Close()
	w.Reset(nil)
	zstdEncoderPool.Put(w.entry)
	return err
}

func (zstdStrategy) NewWriter(dst io.Writer, level, concurrency int) (io.WriteCloser, error) {
	if level < 1 || level > 22 {
		return nil, fmt.Errorf("invalid zstd compression level: %d", level)
	}

	entryAny := zstdEncoderPool.Get()
	var entry *pooledZstdEncoder
	if entryAny == nil {
		entry = &pooledZstdEncoder{}
	} else {
		var ok bool
		entry, ok = entryAny.(*pooledZstdEncoder)
		if !ok {
			return nil, fmt.Errorf("invalid zstd encoder entry type")
		}
	}

	if entry.enc == nil || entry.level != level || entry.concurrency != concurrency {
		encLevel := zstd.EncoderLevelFromZstd(level)
		opts := []zstd.EOption{zstd.WithEncoderLevel(encLevel), zstd.WithEncoderConcurrency(concurrency)}
		enc, err := zstd.NewWriter(dst, opts...)
		if err != nil {
			zstdEncoderPool.Put(entry)
			return nil, err
		}
		entry.enc = enc
		entry.level = level
		entry.concurrency = concurrency
	} else {
		entry.enc.Reset(dst)
	}

	return &pooledZstdWriter{Encoder: entry.enc, entry: entry}, nil
}

type pooledZstdDecoder struct {
	dec         *zstd.Decoder
	concurrency int
}

type pooledZstdReader struct {
	*zstd.Decoder
	entry *pooledZstdDecoder
}

func (zstdStrategy) NewReader(src io.Reader, concurrency int) (io.ReadCloser, error) {
	entryAny := zstdDecoderPool.Get()
	var entry *pooledZstdDecoder
	if entryAny == nil {
		entry = &pooledZstdDecoder{}
	} else {
		var ok bool
		entry, ok = entryAny.(*pooledZstdDecoder)
		if !ok {
			return nil, fmt.Errorf("invalid zstd decoder entry type")
		}
	}

	if entry.dec == nil || entry.concurrency != concurrency {
		if entry.dec != nil {
			entry.dec.Close()
		}
		dec, err := zstd.NewReader(src, zstd.WithDecoderConcurrency(concurrency))
		if err != nil {
			zstdDecoderPool.Put(entry)
			return nil, fmt.Errorf("failed to initialize zstd decoder: %w", err)
		}
		entry.dec = dec
		entry.concurrency = concurrency
	} else {
		if err := entry.dec.Reset(src); err != nil {
			zstdDecoderPool.Put(entry)
			return nil, fmt.Errorf("failed to reset zstd decoder: %w", err)
		}
	}

	return &pooledZstdReader{Decoder: entry.dec, entry: entry}, nil
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

func (r *pooledZstdReader) Close() error {
	if err := r.Reset(nil); err != nil {
		zstdDecoderPool.Put(r.entry)
		return err
	}
	zstdDecoderPool.Put(r.entry)
	return nil
}
