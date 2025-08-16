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

	"lvmsync_go/internal/digest"
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
	data := []byte("hello world")
	algs := []string{digest.BLAKE3, digest.SHA256}
	for _, alg := range algs {
		t.Run(alg, func(t *testing.T) {
			c1, c2 := net.Pipe()
			defer c1.Close()
			defer c2.Close()

			dev := &memDevice{}
			expect, err := digest.SumReader(bytes.NewReader(data), alg)
			if err != nil {
				t.Fatalf("SumReader: %v", err)
			}
			srv := New(dev, alg, expect, zap.NewNop())
			ctx := context.Background()
			errCh := make(chan error)
			go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2)) }()

			cl := rsynkwire.NewClient(rsynkwire.NewStream(c1))
			head, err := cl.SendSignatures(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("SendSignatures: %v", err)
			}
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
			if string(dev.buf) != string(data) {
				t.Fatalf("unexpected buffer %q", string(dev.buf))
			}
			if !dev.sync {
				t.Fatalf("Sync not called")
			}
			if srv.sigHead == nil {
				t.Fatalf("signatures not parsed")
			}
			if srv.sigHead.ChecksumCount != head.ChecksumCount || srv.sigHead.BlockLength != head.BlockLength {
				t.Fatalf("unexpected signature head %+v want %+v", srv.sigHead, head)
			}
			if len(srv.sigHead.Sums) != len(head.Sums) || srv.sigHead.Sums[0].Sum1 != head.Sums[0].Sum1 {
				t.Fatalf("signature sums mismatch")
			}
		})
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
	srv := New(dev, digest.SHA256, [32]byte{}, zap.NewNop())
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

func TestHandleDigestMismatch(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	dev := &memDevice{}
	wrong := [32]byte{}
	srv := New(dev, digest.SHA256, wrong, zap.NewNop())
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2)) }()

	cl := rsynkwire.NewClient(rsynkwire.NewStream(c1))
	data := []byte("mismatch")
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
