package rsynkserver

import (
	"bytes"
	"context"
	"encoding/binary"
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
	buf   []byte
	sync  bool
	short bool
}

func (m *memDevice) WriteAt(p []byte, off int64) (int, error) {
	end := int(off) + len(p)
	if end > len(m.buf) {
		newBuf := make([]byte, end)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	if m.short {
		copy(m.buf[off:], p[:len(p)-1])
		return len(p) - 1, nil
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

func newServer(t *testing.T, dev *memDevice, data []byte) *Server {
	t.Helper()
	srv := New(dev, zap.NewNop())
	exp, err := digest.SumReader(bytes.NewReader(data), digest.SHA256)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	srv.alg = digest.SHA256
	srv.expect = exp
	return srv
}

func TestHandleApplyDelta(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	data := []byte("hello")
	dev := &memDevice{buf: make([]byte, len(data))}
	srv := newServer(t, dev, data)
	ctx := context.Background()
	errCh := make(chan error, 1)
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
		t.Fatalf("device %q want %q", dev.buf, data)
	}
	if !dev.sync {
		t.Fatalf("expected sync")
	}
}

func TestHandleShortWrite(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 2), short: true}
	srv := newServer(t, dev, []byte("hi"))
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1, maxFrame))
	if err := cl.SendDelta(0, []byte("hi")); err != nil {
		t.Fatalf("SendDelta: %v", err)
	}
	c1.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("expected short write error, got %v", err)
	}
}

func TestHandleRejectsOversizedSignatures(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{buf: make([]byte, 1)}
	srv := New(dev, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2, maxFrame)) }()

	var buf bytes.Buffer
	buf.WriteByte('S')
	binary.Write(&buf, binary.LittleEndian, int32(2))
	binary.Write(&buf, binary.LittleEndian, int32(1))
	binary.Write(&buf, binary.LittleEndian, int32(1))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	buf.WriteByte(0)
	binary.Write(&buf, binary.LittleEndian, int32(0))
	buf.WriteByte(0)
	if err := rsynkwire.NewStream(c1, maxFrame).Send(buf.Bytes()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	c1.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "checksum count") {
		t.Fatalf("expected checksum count error, got %v", err)
	}
}
