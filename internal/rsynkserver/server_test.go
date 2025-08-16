package rsynkserver

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsynkwire"
)

type memDevice struct {
	buf  []byte
	sync bool
}

func (m *memDevice) WriteAt(p []byte, off int64) (int, error) {
	copy(m.buf[off:], p)
	return len(p), nil
}

func (m *memDevice) ReadAt(p []byte, off int64) (int, error) {
	n := copy(p, m.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *memDevice) Size() int64 { return int64(len(m.buf)) }

func (m *memDevice) Sync() error { m.sync = true; return nil }

func newServer(t *testing.T, dev *memDevice, expect []byte, logger *zap.Logger) *Server {
	t.Helper()
	h, err := digest.SumReader(bytes.NewReader(expect), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	return New(dev, digest.SHA256, h, logger)
}

func runDelta(t *testing.T, srv *Server, offset int64, data []byte) error {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, 1<<20)) }()
	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, 1<<20))
	if err := cl.SendDelta(offset, data); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	return <-errCh
}

func TestDeltaWithinBounds(t *testing.T) {
	dev := &memDevice{buf: make([]byte, 10)}
	expected := make([]byte, 10)
	copy(expected[3:], []byte("ok"))
	srv := newServer(t, dev, expected, zap.NewNop())
	if err := runDelta(t, srv, 3, []byte("ok")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !bytes.Equal(dev.buf, expected) {
		t.Fatalf("device mismatch: %q != %q", dev.buf, expected)
	}
}

func TestDeltaAtBoundary(t *testing.T) {
	dev := &memDevice{buf: make([]byte, 5)}
	expected := make([]byte, 5)
	copy(expected[3:], []byte("hi"))
	srv := newServer(t, dev, expected, zap.NewNop())
	if err := runDelta(t, srv, 3, []byte("hi")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !bytes.Equal(dev.buf, expected) {
		t.Fatalf("device mismatch: %q != %q", dev.buf, expected)
	}
}

func TestDeltaOverflow(t *testing.T) {
	dev := &memDevice{buf: make([]byte, 5)}
	core, logs := observer.New(zap.WarnLevel)
	srv := newServer(t, dev, dev.buf, zap.New(core))
	err := runDelta(t, srv, 4, []byte("toolong"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if logs.Len() == 0 {
		t.Fatalf("expected warning")
	}
}

func TestDeltaNegativeOffset(t *testing.T) {
	dev := &memDevice{buf: make([]byte, 5)}
	core, logs := observer.New(zap.WarnLevel)
	srv := newServer(t, dev, dev.buf, zap.New(core))
	err := runDelta(t, srv, -1, []byte("x"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if logs.Len() == 0 {
		t.Fatalf("expected warning")
	}
}
