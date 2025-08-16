package rsynkserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsynkwire"
)

const maxFrame = 1 << 20

type memDevice struct {
	buf         []byte
	sync        bool
	fail        bool
	writeCalled bool
}

func (m *memDevice) WriteAt(p []byte, off int64) (int, error) {
	m.writeCalled = true
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

func TestHandleApplyDelta(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	data := []byte("hello")
	exp, err := digest.SumReader(bytes.NewReader(data), digest.SHA256)
	if err != nil {
		t.Fatalf("SumReader: %v", err)
	}

	dev := &memDevice{buf: make([]byte, len(data))}
	srv := New(dev, digest.SHA256, exp, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	if err := cl.SendDelta(0, data); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if string(dev.buf) != string(data) {
		t.Fatalf("expected %q, got %q", data, dev.buf)
	}
	if !dev.sync {
		t.Fatalf("device not synced")
	}
}

func TestHandleCRCError(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{}
	srv := New(dev, digest.SHA256, [32]byte{}, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	payload := make([]byte, 1+8)
	payload[0] = 'D'
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(hdr[4:8], 0) // invalid CRC
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
	srv := New(dev, digest.SHA256, [32]byte{}, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	if err := cl.SendDelta(0, []byte("data")); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected write error")
	}
}

func TestHandleDigestMismatch(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	data := []byte("mismatch")
	dev := &memDevice{buf: make([]byte, len(data))}
	wrong := [32]byte{}
	srv := New(dev, digest.SHA256, wrong, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	if _, err := cl.SendSignatures(bytes.NewReader(data)); err != nil {
		t.Fatalf("SendSignatures: %v", err)
	}
	if err := cl.SendDelta(0, data); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

func TestHandleDeltaOutOfBounds(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 8)}
	core, logs := observer.New(zap.ErrorLevel)
	srv := New(dev, digest.SHA256, [32]byte{}, zap.New(core))
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	if err := cl.SendDelta(6, []byte("abcd")); err != nil { // 6+4=10 > 8
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "exceeds device size") {
		t.Fatalf("expected out-of-bounds error, got %v", err)
	}
	if dev.writeCalled {
		t.Fatalf("device write should not be called")
	}
	if logs.Len() == 0 || logs.All()[0].Message != "delta_out_of_bounds" {
		t.Fatalf("expected delta_out_of_bounds log, got %v", logs.All())
	}
}
