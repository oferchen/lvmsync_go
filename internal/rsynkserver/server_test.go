package rsynkserver

import (
	"bytes"
	"context"
	"io"
	"math"
	"net"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsynkwire"
)

type memDevice struct {
	buf         []byte
	sync        bool
	fail        bool
	writeCalled bool
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

func TestHandleApplyDelta(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{}
	expect, err := digest.SumReader(bytes.NewReader([]byte("hello")), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	srv := New(dev, digest.SHA256, expect, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	if err := cl.SendDelta(0, []byte("hello")); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("Handle: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := dev.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("data %q want %q", string(buf), "hello")
	}
	if dev.Size() != 5 {
		t.Fatalf("size %d want 5", dev.Size())
	}
	if !dev.sync {
		t.Fatalf("expected sync called")
	}
}


func TestHandleCRCError(t *testing.T) {

func (m *memDevice) Size() int64 { return int64(len(m.buf)) }

func (m *memDevice) Sync() error { m.sync = true; return nil }

func TestHandleDigestSuccess(t *testing.T) {
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

	payload := make([]byte, 1+8)
	payload[0] = 'D'
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(hdr[4:8], 0)
	if _, err := c1.Write(hdr[:]); err != nil {
		t.Fatalf("write hdr: %v", err)
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

func runDelta(t *testing.T, srv *Server, offset int64, data []byte) error {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	if err := cl.SendDelta(0, []byte("data")); err != nil {
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

	dev := &memDevice{}
	wrong := [32]byte{}
	srv := New(dev, digest.SHA256, wrong, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	data := []byte("mismatch")
	if _, err := cl.SendSignatures(bytes.NewReader(data)); err != nil {
		t.Fatalf("SendSignatures: %v", err)
	}
	if err := cl.SendDelta(0, data); err != nil {
		t.Fatalf("SendDelta: %v", err)


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
	ctxFields := logs.All()[0].ContextMap()
	if ctxFields["offset_bytes"].(int64) != 6 ||
		ctxFields["delta_size_bytes"].(int64) != 4 ||
		ctxFields["end_offset_bytes"].(int64) != 10 ||
		ctxFields["device_size_bytes"].(int64) != 8 {
		t.Fatalf("unexpected log fields: %v", ctxFields)
	}
}

func TestHandleDeltaOffsetOverflow(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 8)}
	core, logs := observer.New(zap.ErrorLevel)
	srv := New(dev, digest.SHA256, [32]byte{}, zap.New(core))
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	stream := rsynkwire.NewStream(c1, maxFrame)
	var buf bytes.Buffer
	buf.WriteByte('D')
	var offBytes [8]byte
	binary.BigEndian.PutUint64(offBytes[:], uint64(math.MaxInt64)+1)
	buf.Write(offBytes[:])
	buf.WriteByte('x')
	if err := stream.Send(buf.Bytes()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	c1.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "overflows int64") {
		t.Fatalf("expected overflow error, got %v", err)
	}
	if dev.writeCalled {
		t.Fatalf("device write should not be called")
	}
	if logs.Len() == 0 || logs.All()[0].Message != "delta_out_of_bounds" {
		t.Fatalf("expected delta_out_of_bounds log, got %v", logs.All())
	}
	ctxFields := logs.All()[0].ContextMap()
	if ctxFields["offset_bytes"].(uint64) != uint64(math.MaxInt64)+1 ||
		ctxFields["delta_size_bytes"].(int64) != 1 ||
		ctxFields["device_size_bytes"].(int64) != 8 {
		t.Fatalf("unexpected log fields: %v", ctxFields)
	}
}
