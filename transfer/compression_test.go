package transfer

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	zstd "github.com/klauspost/compress/zstd"
)

func TestCompressionRoundTrip(t *testing.T) {
	data := []byte("some test data for compression")
	types := []string{"none", compressionLZ4, compressionZSTD}

	for _, c := range types {
		var buf bytes.Buffer
		w, err := NewCompressionWriter(&buf, c, 1, 1)
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
		if _, err := NewCompressionWriter(io.Discard, compressionZSTD, 3, 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if _, err := NewCompressionWriter(io.Discard, compressionZSTD, 100, 1); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestNewCompressionWriterConcurrency(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewCompressionWriter(&buf, compressionZSTD, 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer w.Close()
	enc := w.(*zstd.Encoder)
	conc := int(reflect.ValueOf(enc).Elem().FieldByName("o").FieldByName("concurrent").Int())
	if conc != 2 {
		t.Fatalf("expected concurrency 2, got %d", conc)
	}
}
