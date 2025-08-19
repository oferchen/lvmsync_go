package transfer

import (
	"bytes"
	lz4 "github.com/pierrec/lz4/v4"
	"testing"
)

func TestCompressionFrameHeaders(t *testing.T) {
	data := bytes.Repeat([]byte{'a'}, 1<<10)

	// Zstd default
	var buf bytes.Buffer
	w, err := NewCompressionWriter(&buf, compressionZSTD, 1, 1)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("zstd write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zstd close: %v", err)
	}
	out := buf.Bytes()
	wantZstd := []byte{0x28, 0xB5, 0x2F, 0xFD}
	if len(out) < len(wantZstd) || !bytes.Equal(out[:len(wantZstd)], wantZstd) {
		t.Fatalf("zstd magic mismatch: % x", out[:len(wantZstd)])
	}

	// LZ4 fallback
	buf.Reset()
	w, err = NewCompressionWriter(&buf, compressionLZ4, int(lz4.Level1), 1)
	if err != nil {
		t.Fatalf("lz4 writer: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("lz4 write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("lz4 close: %v", err)
	}
	out = buf.Bytes()
	wantLZ4 := []byte{0x04, 0x22, 0x4D, 0x18}
	if len(out) < len(wantLZ4) || !bytes.Equal(out[:len(wantLZ4)], wantLZ4) {
		t.Fatalf("lz4 magic mismatch: % x", out[:len(wantLZ4)])
	}
}
