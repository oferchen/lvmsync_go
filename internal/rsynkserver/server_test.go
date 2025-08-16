package rsynkserver

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsynkwire"
)

const maxFrame = 1 << 20

type memDevice struct {
	buf  []byte
	sync bool
}

func (m *memDevice) WriteAt(p []byte, off int64) (int, error) {
	end := int(off) + len(p)
	if end > len(m.buf) {
		newBuf := make([]byte, end)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	copy(m.buf[off:], p)
	return len(p), nil
}

func (m *memDevice) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(m.buf)) {
		return 0, io.EOF
	}
	n := copy(p, m.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *memDevice) Size() int64 { return int64(len(m.buf)) }

func (m *memDevice) Sync() error { m.sync = true; return nil }

func TestHandleDigestSuccess(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{}
	srv := New(dev, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	data := []byte("hello")
	if _, err := cl.SendSignatures(bytes.NewReader(data)); err != nil {
		t.Fatalf("SendSignatures: %v", err)
	}
	if err := cl.SendDelta(0, data); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	sum, err := digest.SumReader(bytes.NewReader(data), digest.SHA256)
	if err != nil {
		t.Fatalf("SumReader: %v", err)
	}
	if err := cl.SendDigest(digest.SHA256, sum); err != nil {
		t.Fatalf("SendDigest: %v", err)
	}
	c1.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if string(dev.buf) != "hello" {
		t.Fatalf("unexpected device contents: %q", dev.buf)
	}
}

func TestHandleDigestMismatch(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{}
	srv := New(dev, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	data := []byte("hello")
	if _, err := cl.SendSignatures(bytes.NewReader(data)); err != nil {
		t.Fatalf("SendSignatures: %v", err)
	}
	if err := cl.SendDelta(0, data); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	var wrong [32]byte
	if err := cl.SendDigest(digest.SHA256, wrong); err != nil {
		t.Fatalf("SendDigest: %v", err)
	}
	c1.Close()
	err := <-errCh
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
