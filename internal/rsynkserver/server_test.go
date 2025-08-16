package rsynkserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"

	"lvmsync_go/internal/rsynkwire"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestHandleGood(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	buf := &bytes.Buffer{}
	srv := New(buf)
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2)) }()

	client := rsynkwire.NewStream(c1)
	if err := client.Send([]byte("hello")); err != nil {
		t.Fatalf("send: %v", err)
	}
	c1.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := buf.String(); got != "hello" {
		t.Fatalf("unexpected write %q", got)
	}
}

func TestHandleBadCRC(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	buf := &bytes.Buffer{}
	srv := New(buf)
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2)) }()

	// craft frame with incorrect CRC
	payload := []byte("data")
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

	srv := New(errWriter{})
	ctx := context.Background()
	errCh := make(chan error)
	go func() { errCh <- srv.Handle(ctx, rsynkwire.NewStream(c2)) }()

	client := rsynkwire.NewStream(c1)
	if err := client.Send([]byte("chunk")); err != nil {
		t.Fatalf("send: %v", err)
	}
	c1.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected write error")
	}
}
