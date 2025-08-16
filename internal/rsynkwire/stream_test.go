package rsynkwire

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"net"
	"strings"
	"testing"
)

func TestStreamRecvWithinLimit(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	const max = 4
	sender := NewStream(c1, max)
	recv := NewStream(c2, max)

	payload := []byte("test")
	go func() {
		if err := sender.Send(payload); err != nil {
			t.Errorf("Send: %v", err)
		}
	}()

	frame, err := recv.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if string(frame) != string(payload) {
		t.Fatalf("unexpected payload %q", string(frame))
	}
}

func TestStreamRecvTooLarge(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	const max = 4
	recv := NewStream(c2, max)

	payload := []byte("hello")
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(hdr[4:8], crc32.Checksum(payload, crcTable))
	go func() {
		c1.Write(hdr[:])
	}()

	if _, err := recv.Recv(); err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected size limit error, got %v", err)
	}
}

func TestStreamSendTooLarge(t *testing.T) {
	var buf bytes.Buffer
	const max = 4
	s := NewStream(&buf, max)
	payload := []byte("hello")
	if err := s.Send(payload); err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected size limit error, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no data written, got %d bytes", buf.Len())
	}
}
