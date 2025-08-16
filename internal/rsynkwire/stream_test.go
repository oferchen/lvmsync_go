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

type shortReadWriter struct {
	bytes.Buffer
	max   int
	calls int
}

func (s *shortReadWriter) Write(p []byte) (int, error) {
	s.calls++
	if len(p) > s.max {
		p = p[:s.max]
	}
	return s.Buffer.Write(p)
}

// TestStreamSendShortWrites ensures Send retries until all bytes are written.
func TestStreamSendShortWrites(t *testing.T) {
	const limit = 3
	srw := &shortReadWriter{max: limit}
	s := NewStream(srw, 1<<20)
	payload := []byte("hello world")
	if err := s.Send(payload); err != nil {
		t.Fatalf("Send: %v", err)
	}
	expectedLen := 8 + len(payload)
	if srw.Len() != expectedLen {
		t.Fatalf("wrote %d bytes, want %d", srw.Len(), expectedLen)
	}
	data := srw.Bytes()
	n := binary.BigEndian.Uint32(data[0:4])
	if int(n) != len(payload) {
		t.Fatalf("header length %d != payload length %d", n, len(payload))
	}
	crc := binary.BigEndian.Uint32(data[4:8])
	if crc != crc32.Checksum(payload, crcTable) {
		t.Fatalf("crc mismatch")
	}
	// Expect multiple writes due to short writer.
	expectedCalls := (expectedLen + limit - 1) / limit
	if srw.calls != expectedCalls {
		t.Fatalf("expected %d writes, got %d", expectedCalls, srw.calls)
	}
}
