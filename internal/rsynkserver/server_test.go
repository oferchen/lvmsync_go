package rsynkserver

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"

	"lvmsync_go/internal/rsynkwire"
)

type memDevice struct {
	buf  []byte
	sync bool
	fail bool
}

func (m *memDevice) WriteAt(p []byte, off int64) (int, error) {
	if m.fail {
		return 0, errors.New("write failed")
	}
	end := int(off) + len(p)
	if end > len(m.buf) {
		newBuf := make([]byte, end)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	copy(m.buf[off:], p)
	return len(p), nil
}

func (m *memDevice) Sync() error { m.sync = true; return nil }

func TestHandleApplyDelta(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{}
	srv := New(dev)
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1))
	if err := cl.SendDelta(0, []byte("hello")); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	if err := cl.SendDelta(5, []byte(" world")); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if string(dev.buf) != "hello world" {
		t.Fatalf("unexpected buffer %q", string(dev.buf))
	}
	if !dev.sync {
		t.Fatalf("Sync not called")
	}
}

func TestHandleCRCError(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{}
	srv := New(dev)
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2)) }()

	// craft invalid CRC frame
	payload := make([]byte, 1+8) // 'D' + offset 0
	payload[0] = 'D'
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(hdr[4:8], 0) // wrong CRC
	if _, err := c1.Write(hdr[:]); err != nil {
		t.Fatalf("write hdr: %v", err)
	}
	if _, err := c1.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	c1.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleWriteError(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{fail: true}
	srv := New(dev)
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1))
	if err := cl.SendDelta(0, []byte("data")); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected write error")
	}
}
