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

func TestLZ4WriterPoolReuse(t *testing.T) {
	data1 := []byte("first payload")
	data2 := []byte("second payload")

	var buf bytes.Buffer
	w1, err := NewCompressionWriter(&buf, compressionLZ4, int(lz4.Level3), 1)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	if _, err := w1.Write(data1); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("close1: %v", err)
	}

	pw1 := w1.(*pooledLz4Writer)
	writerPtr := pw1.Writer

	r1, err := NewDecompressionReader(&buf, compressionLZ4, 1)
	if err != nil {
		t.Fatalf("reader1: %v", err)
	}
	out1, err := io.ReadAll(r1)
	if err != nil {
		t.Fatalf("read1: %v", err)
	}
	if err := r1.Close(); err != nil {
		t.Fatalf("close reader1: %v", err)
	}
	if !bytes.Equal(out1, data1) {
		t.Fatalf("mismatch1")
	}

	buf.Reset()
	w2, err := NewCompressionWriter(&buf, compressionLZ4, int(lz4.Level3), 1)
	if err != nil {
		t.Fatalf("new writer2: %v", err)
	}
	if _, err := w2.Write(data2); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}

	pw2 := w2.(*pooledLz4Writer)
	if pw2.Writer != writerPtr {
		t.Fatalf("writer was not reused")
	}

	r2, err := NewDecompressionReader(&buf, compressionLZ4, 1)
	if err != nil {
		t.Fatalf("reader2: %v", err)
	}
	out2, err := io.ReadAll(r2)
	if err != nil {
		t.Fatalf("read2: %v", err)
	}
	if err := r2.Close(); err != nil {
		t.Fatalf("close reader2: %v", err)
	}
	if !bytes.Equal(out2, data2) {
		t.Fatalf("mismatch2")
	}
}

func TestLZ4ReaderPoolReuse(t *testing.T) {
	data1 := []byte("alpha")
	data2 := []byte("beta")

	compress := func(data []byte) []byte {
		var b bytes.Buffer
		w, err := NewCompressionWriter(&b, compressionLZ4, int(lz4.Level3), 1)
		if err != nil {
			t.Fatalf("compress writer: %v", err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("compress write: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("compress close: %v", err)
		}
		return b.Bytes()
	}

	buf1 := bytes.NewBuffer(compress(data1))
	r1, err := NewDecompressionReader(buf1, compressionLZ4, 1)
	if err != nil {
		t.Fatalf("new reader1: %v", err)
	}
	out1, err := io.ReadAll(r1)
	if err != nil {
		t.Fatalf("read1: %v", err)
	}
	if err := r1.Close(); err != nil {
		t.Fatalf("close1: %v", err)
	}
	if !bytes.Equal(out1, data1) {
		t.Fatalf("mismatch1")
	}

	pr1 := r1.(*pooledLz4Reader)
	readerPtr := pr1.Reader

	buf2 := bytes.NewBuffer(compress(data2))
	r2, err := NewDecompressionReader(buf2, compressionLZ4, 1)
	if err != nil {
		t.Fatalf("new reader2: %v", err)
	}
	if r2.(*pooledLz4Reader).Reader != readerPtr {
		t.Fatalf("reader was not reused")
	}
	out2, err := io.ReadAll(r2)
	if err != nil {
		t.Fatalf("read2: %v", err)
	}
	if err := r2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
	if !bytes.Equal(out2, data2) {
		t.Fatalf("mismatch2")
	}
}
