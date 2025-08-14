package quic

import (
	"context"
	"io"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func checkLogFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int) {
	entries := logs.FilterMessage(msg).All()
	if len(entries) != expected {
		t.Fatalf("expected %d %s logs, got %d", expected, msg, len(entries))
	}
	ctx := entries[0].ContextMap()
	for _, k := range []string{"address", "role", "duration_ms", "error"} {
		if _, ok := ctx[k]; !ok {
			t.Fatalf("expected field %q in %s log", k, msg)
		}
	}
}

func TestQUICTransportHandshake(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core)})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
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
		qconn := conn.(*Conn)
		if _, err := tr.Negotiate(ctx, qconn, transport.Server, common.Handshake{}); err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		dctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		msg, err := qconn.ReceiveDatagram(dctx)
		if err != nil {
			t.Errorf("receive datagram: %v", err)
			return
		}
		if string(msg) != "hello" {
			t.Errorf("unexpected datagram %q", msg)
		}
		if err := qconn.SendDatagram([]byte("world")); err != nil {
			t.Errorf("send datagram: %v", err)
			return
		}
		buf := make([]byte, 4)
		io.ReadFull(qconn, buf)
		qconn.Write([]byte("pong"))
		qconn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	if _, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{}); err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if err := qconn.SendDatagram([]byte("hello")); err != nil {
		t.Fatalf("send datagram: %v", err)
	}
	dctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	msg, err := qconn.ReceiveDatagram(dctx)
	if err != nil {
		t.Fatalf("receive datagram: %v", err)
	}
	if string(msg) != "world" {
		t.Fatalf("unexpected datagram %q", msg)
	}
	if _, err := qconn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	io.ReadFull(qconn, buf)
	if string(buf) != "pong" {
		t.Fatalf("unexpected response %q", buf)
	}
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1)
	checkLogFields(t, logs, "dial_end", 1)
	checkLogFields(t, logs, "listen_start", 1)
	checkLogFields(t, logs, "listen_end", 1)
	checkLogFields(t, logs, "negotiate_start", 2)
	checkLogFields(t, logs, "negotiate_end", 2)
}

func TestQUICTransportHandshakeError(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core)})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
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
		qconn := conn.(*Conn)
		qconn.Write([]byte("bad\n"))
		qconn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	if _, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{}); err == nil {
		t.Fatalf("expected negotiate error")
	}
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1)
	checkLogFields(t, logs, "dial_end", 1)
	checkLogFields(t, logs, "listen_start", 1)
	checkLogFields(t, logs, "listen_end", 1)
	checkLogFields(t, logs, "negotiate_start", 1)
	checkLogFields(t, logs, "negotiate_end", 1)
}

func TestQUICTransportRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}
