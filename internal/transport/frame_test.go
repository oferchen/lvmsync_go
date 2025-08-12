package transport

import (
	"bytes"
	"testing"
)

func TestFrameHandshake(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("hello")
	if err := WriteFrame(&buf, 1, FlagHandshake, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	f, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if f.Index != 1 || f.Flags != FlagHandshake {
		t.Fatalf("unexpected frame header: %+v", f)
	}
	if !bytes.Equal(f.Payload, payload) {
		t.Fatalf("payload mismatch: %q != %q", f.Payload, payload)
	}
}

func TestFrameRetransmit(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("retransmit")
	if err := WriteFrame(&buf, 2, 0, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	data := buf.Bytes()
	data[len(data)-1] ^= 0xff // corrupt payload
	buf2 := bytes.NewBuffer(data)
	if _, err := ReadFrame(buf2); err == nil {
		t.Fatalf("expected checksum mismatch")
	}
	buf.Reset()
	if err := WriteFrame(&buf, 2, 0, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if _, err := ReadFrame(&buf); err != nil {
		t.Fatalf("ReadFrame after retransmit: %v", err)
	}
}

func TestFrameBitmapSync(t *testing.T) {
	dst := []byte{0x00, 0x00}
	var buf bytes.Buffer
	payload := []byte{0x01, 0x10}
	if err := WriteFrame(&buf, 3, FlagBitmap, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	f, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	ApplyBitmap(dst, f)
	if dst[0] != 0x01 || dst[1] != 0x10 {
		t.Fatalf("bitmap not synced: %v", dst)
	}
}
