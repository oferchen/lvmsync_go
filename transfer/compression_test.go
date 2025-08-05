package transfer

import (
	"bytes"
	"io"
	"testing"

	"github.com/pierrec/lz4/v4"
)

func TestCompressionRoundTrip(t *testing.T) {
	data := []byte("some test data for compression")
	cases := []struct {
		c     string
		level int
	}{
		{"none", 0},
		{compressionLZ4, int(lz4.Level1)},
		{compressionZSTD, 1},
	}

	for _, tc := range cases {
		var buf bytes.Buffer
		w, err := NewCompressionWriter(&buf, tc.c, tc.level, 1)
		if err != nil {
			t.Fatalf("writer for %s: %v", tc.c, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write %s: %v", tc.c, err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close %s: %v", tc.c, err)
		}

		r, err := NewDecompressionReader(&buf, tc.c, 1)
		if err != nil {
			t.Fatalf("reader for %s: %v", tc.c, err)
		}
		out, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read %s: %v", tc.c, err)
		}
		if err := r.Close(); err != nil {
			t.Fatalf("close %s: %v", tc.c, err)
		}

		if !bytes.Equal(out, data) {
			t.Fatalf("roundtrip mismatch for %s", tc.c)
		}
	}
}

func TestLZ4CompressionLevels(t *testing.T) {
	data := []byte("some test data for compression")
	levels := []int{int(lz4.Fast), int(lz4.Level5), int(lz4.Level9)}

	for _, lvl := range levels {
		var buf bytes.Buffer
		w, err := NewCompressionWriter(&buf, compressionLZ4, lvl, 1)
		if err != nil {
			t.Fatalf("writer for level %d: %v", lvl, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write level %d: %v", lvl, err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close level %d: %v", lvl, err)
		}

		r, err := NewDecompressionReader(&buf, compressionLZ4, 1)
		if err != nil {
			t.Fatalf("reader for level %d: %v", lvl, err)
		}
		out, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read level %d: %v", lvl, err)
		}
		if err := r.Close(); err != nil {
			t.Fatalf("close level %d: %v", lvl, err)
		}

		if !bytes.Equal(out, data) {
			t.Fatalf("roundtrip mismatch for level %d", lvl)
		}
	}
}

func TestNewCompressionWriterLevel(t *testing.T) {
	t.Run("zstdValid", func(t *testing.T) {
		if _, err := NewCompressionWriter(io.Discard, compressionZSTD, 3, 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("zstdInvalid", func(t *testing.T) {
		if _, err := NewCompressionWriter(io.Discard, compressionZSTD, 100, 1); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("lz4Valid", func(t *testing.T) {
		if _, err := NewCompressionWriter(io.Discard, compressionLZ4, int(lz4.Level3), 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("lz4Invalid", func(t *testing.T) {
		if _, err := NewCompressionWriter(io.Discard, compressionLZ4, 3, 1); err == nil {
			t.Fatalf("expected error")
		}
	})
}
