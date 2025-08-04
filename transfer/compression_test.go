package transfer

import (
	"bytes"
	"io"
	"testing"
)

func TestCompressionRoundTrip(t *testing.T) {
	data := []byte("some test data for compression")
	types := []string{"none", compressionLZ4, compressionZSTD}

	for _, c := range types {
		var buf bytes.Buffer
		w, err := NewCompressionWriter(&buf, c, 1)
		if err != nil {
			t.Fatalf("writer for %s: %v", c, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write %s: %v", c, err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close %s: %v", c, err)
		}

		r, err := NewDecompressionReader(&buf, c)
		if err != nil {
			t.Fatalf("reader for %s: %v", c, err)
		}
		out, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read %s: %v", c, err)
		}
		r.Close()

		if !bytes.Equal(out, data) {
			t.Fatalf("roundtrip mismatch for %s", c)
		}
	}
}

func TestNewCompressionWriterLevel(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		if _, err := NewCompressionWriter(io.Discard, compressionZSTD, 3); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if _, err := NewCompressionWriter(io.Discard, compressionZSTD, 100); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestCompressionPooling(t *testing.T) {
	for _, c := range []string{compressionLZ4, compressionZSTD} {
		// Writer reuse
		w1, err := NewCompressionWriter(io.Discard, c, 1)
		if err != nil {
			t.Fatalf("writer1 for %s: %v", c, err)
		}
		var firstWriter any
		switch w := w1.(type) {
		case *lz4WriteCloser:
			firstWriter = w.Writer
		case *zstdWriteCloser:
			firstWriter = w.Encoder
		}
		w1.Close()

		w2, err := NewCompressionWriter(io.Discard, c, 1)
		if err != nil {
			t.Fatalf("writer2 for %s: %v", c, err)
		}
		var secondWriter any
		switch w := w2.(type) {
		case *lz4WriteCloser:
			secondWriter = w.Writer
		case *zstdWriteCloser:
			secondWriter = w.Encoder
		}
		if firstWriter != secondWriter {
			t.Fatalf("writer was not pooled for %s", c)
		}
		w2.Close()

		// Reader reuse
		r1, err := NewDecompressionReader(bytes.NewReader(nil), c)
		if err != nil {
			t.Fatalf("reader1 for %s: %v", c, err)
		}
		var firstReader any
		switch r := r1.(type) {
		case *lz4ReadCloser:
			firstReader = r.Reader
		case *zstdReadCloser:
			firstReader = r.Decoder
		}
		r1.Close()

		r2, err := NewDecompressionReader(bytes.NewReader(nil), c)
		if err != nil {
			t.Fatalf("reader2 for %s: %v", c, err)
		}
		var secondReader any
		switch r := r2.(type) {
		case *lz4ReadCloser:
			secondReader = r.Reader
		case *zstdReadCloser:
			secondReader = r.Decoder
		}
		if firstReader != secondReader {
			t.Fatalf("reader was not pooled for %s", c)
		}
		r2.Close()
	}
}
