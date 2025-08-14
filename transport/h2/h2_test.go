package h2

import (
	"context"
	"io"
	"testing"

	"go.uber.org/zap"
	"lvmsync_go/common"
	"lvmsync_go/transport"

	"go.uber.org/zap/zaptest"
)

func TestH2TransportHandshake(t *testing.T) {
	trIface, _ := New(transport.Config{Logger: zap.NewNop()})
	tr := trIface.(*Transport)
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		if _, err := tr.Negotiate(ctx, conn, transport.Server, common.Handshake{}); err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		buf := make([]byte, 4)
		io.ReadFull(conn, buf)
		conn.Write([]byte("pong"))
		conn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{}); err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	io.ReadFull(conn, buf)
	if string(buf) != "pong" {
		t.Fatalf("unexpected response %q", buf)
	}
	conn.Close()
	<-done
}

func TestH2TransportHandshakeError(t *testing.T) {
	trIface, _ := New(transport.Config{Logger: zap.NewNop()})
	tr := trIface.(*Transport)
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		// send invalid handshake to trigger failure
		conn.Write([]byte("bad\n"))
		conn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{}); err == nil {
		t.Fatalf("expected negotiate error")
	}
	conn.Close()
	<-done
}
