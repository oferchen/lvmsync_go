package rsynkwire

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestStreamRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	s1 := NewStream(c1)
	s2 := NewStream(c2)

	want := []byte("payload")
	go func() { _ = s1.Send(want); c1.Close() }()

	got, err := s2.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestStreamBadCRC(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	s := NewStream(c2)
	payload := []byte("bad")
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(hdr[4:8], 0) // wrong CRC
	if _, err := c1.Write(hdr[:]); err != nil {
		t.Fatalf("write hdr: %v", err)
	}
	if _, err := c1.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if _, err := s.Recv(); err == nil {
		t.Fatalf("expected CRC error")
	}
}
