package frame

import "testing"

func TestRoundTrip(t *testing.T) {
	f := NewBinary()
	hdr := Header{Version: 1, Flags: 2, Offset: 3, Length: 4, CmpLen: 5}
	copy(hdr.Hash[:], []byte("hashhashhashhashhashhashhashhash"))
	payload := []byte("payload")
	buf, err := f.Encode(hdr, payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	h2, p2, err := f.Decode(buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if h2 != hdr || string(p2) != string(payload) {
		t.Fatalf("mismatch")
	}
}

func TestDecodeShort(t *testing.T) {
	f := NewBinary()
	if _, _, err := f.Decode([]byte{1, 2, 3}); err == nil {
		t.Fatalf("expected error")
	}
}
