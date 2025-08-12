package transport

import (
	"bytes"
	"testing"
)

func TestFrameEncodeDecode(t *testing.T) {
	f := Frame{Index: 1, Flags: FlagHasHash, Hash: bytes.Repeat([]byte{0xab}, 32), Payload: []byte("data")}
	var buf bytes.Buffer
	if err := f.WriteTo(&buf); err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	var out Frame
	if err := out.ReadFrame(&buf); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if out.Index != f.Index || out.Flags != f.Flags || !bytes.Equal(out.Hash, f.Hash) || !bytes.Equal(out.Payload, f.Payload) {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestEncodeFrameHashTooLarge(t *testing.T) {
	var buf bytes.Buffer
	hash := bytes.Repeat([]byte{1}, 33)
	if err := EncodeFrame(&buf, 0, 0, hash, nil); err == nil {
		t.Fatalf("expected error for large hash")
	}
}
