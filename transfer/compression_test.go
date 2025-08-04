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
