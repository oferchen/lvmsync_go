// transfer/compression.go
package transfer

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

const (
	compressionLZ4  = "lz4"
	compressionZSTD = "zstd"
)

// Compressor provides methods to create pooled writers and readers.
type Compressor interface {
	NewWriter(io.Writer, int) (io.WriteCloser, error)
	NewReader(io.Reader) (io.ReadCloser, error)
}

var compressors = map[string]Compressor{
	compressionLZ4:  newLZ4Compressor(),
	compressionZSTD: newZstdCompressor(),
}

// NewCompressionWriter returns a writer for the given compression type. The
// writer is pooled and must be closed to be returned to the pool.
func NewCompressionWriter(w io.Writer, compress string, level int) (io.WriteCloser, error) {
	if compress == "auto" {
		compress = detectOptimalCompression()
	}

	if compress == "none" {
		return nopWriteCloser{w}, nil
	}

	c, ok := compressors[compress]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
	return c.NewWriter(w, level)
}

// NewDecompressionReader returns a reader for the given compression type. The
// reader is pooled and must be closed to be returned to the pool.
func NewDecompressionReader(r io.Reader, compress string) (io.ReadCloser, error) {
	if compress == "none" {
		return io.NopCloser(r), nil
	}

	c, ok := compressors[compress]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type: %s", compress)
	}
	return c.NewReader(r)
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// LZ4 implementation -------------------------------------------------------

type lz4Compressor struct {
	writerPool sync.Pool
	readerPool sync.Pool
}

func newLZ4Compressor() *lz4Compressor {
	return &lz4Compressor{
		writerPool: sync.Pool{New: func() any { return lz4.NewWriter(io.Discard) }},
		readerPool: sync.Pool{New: func() any { return lz4.NewReader(bytes.NewReader(nil)) }},
	}
}

func (c *lz4Compressor) NewWriter(w io.Writer, _ int) (io.WriteCloser, error) {
	lw := c.writerPool.Get().(*lz4.Writer)
	lw.Reset(w)
	return &lz4WriteCloser{Writer: lw, pool: &c.writerPool}, nil
}

func (c *lz4Compressor) NewReader(r io.Reader) (io.ReadCloser, error) {
	lr := c.readerPool.Get().(*lz4.Reader)
	lr.Reset(r)
	return &lz4ReadCloser{Reader: lr, pool: &c.readerPool}, nil
}

type lz4WriteCloser struct {
	*lz4.Writer
	pool *sync.Pool
}

func (w *lz4WriteCloser) Close() error {
	err := w.Writer.Close()
	w.Writer.Reset(io.Discard)
	w.pool.Put(w.Writer)
	return err
}

type lz4ReadCloser struct {
	*lz4.Reader
	pool *sync.Pool
}

func (r *lz4ReadCloser) Close() error {
	r.Reader.Reset(bytes.NewReader(nil))
	r.pool.Put(r.Reader)
	return nil
}

// Zstd implementation ------------------------------------------------------

type zstdCompressor struct {
	mu          sync.Mutex
	writerPools map[int]*sync.Pool
	readerPool  sync.Pool
}

func newZstdCompressor() *zstdCompressor {
	return &zstdCompressor{
		writerPools: make(map[int]*sync.Pool),
		readerPool:  sync.Pool{New: func() any { dec, _ := zstd.NewReader(nil); return dec }},
	}
}

func (c *zstdCompressor) poolForLevel(level int) *sync.Pool {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.writerPools[level]
	if p == nil {
		encLevel := zstd.EncoderLevelFromZstd(level)
		p = &sync.Pool{New: func() any { enc, _ := zstd.NewWriter(io.Discard, zstd.WithEncoderLevel(encLevel)); return enc }}
		c.writerPools[level] = p
	}
	return p
}

func (c *zstdCompressor) NewWriter(w io.Writer, level int) (io.WriteCloser, error) {
	if level < 1 || level > 22 {
		return nil, fmt.Errorf("invalid zstd compression level: %d", level)
	}
	p := c.poolForLevel(level)
	zw := p.Get().(*zstd.Encoder)
	zw.Reset(w)
	return &zstdWriteCloser{Encoder: zw, pool: p}, nil
}

func (c *zstdCompressor) NewReader(r io.Reader) (io.ReadCloser, error) {
	zr := c.readerPool.Get().(*zstd.Decoder)
	if err := zr.Reset(r); err != nil {
		// decoder cannot be reused; discard from pool
		zr.Close()
		return nil, fmt.Errorf("failed to initialize zstd decoder: %w", err)
	}
	return &zstdReadCloser{Decoder: zr, pool: &c.readerPool}, nil
}

type zstdWriteCloser struct {
	*zstd.Encoder
	pool *sync.Pool
}

func (w *zstdWriteCloser) Close() error {
	err := w.Encoder.Close()
	w.Encoder.Reset(nil)
	w.pool.Put(w.Encoder)
	return err
}

type zstdReadCloser struct {
	*zstd.Decoder
	pool *sync.Pool
}

func (r *zstdReadCloser) Close() error {
	r.Decoder.Reset(nil)
	r.pool.Put(r.Decoder)
	return nil
}
