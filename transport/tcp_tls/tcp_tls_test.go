package tcp_tls

import (
	"context"
	"crypto/x509"
	"io"
	"testing"

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

func TestTCPTLSTransportHandshake(t *testing.T) {
	cert, _ := generateSelfSignedCert()
	root := x509.NewCertPool()
	if len(cert.Certificate) > 0 {
		if c, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
			root.AddCert(c)
		}
	}
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert})
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

	checkLogFields(t, logs, "dial_start", 1)
	checkLogFields(t, logs, "dial_end", 1)
	checkLogFields(t, logs, "listen_start", 1)
	checkLogFields(t, logs, "listen_end", 1)
	checkLogFields(t, logs, "negotiate_start", 2)
	checkLogFields(t, logs, "negotiate_end", 2)
}

func TestTCPTLSTransportHandshakeError(t *testing.T) {
	cert, _ := generateSelfSignedCert()
	root := x509.NewCertPool()
	if c, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
		root.AddCert(c)
	}
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert})
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

	checkLogFields(t, logs, "dial_start", 1)
	checkLogFields(t, logs, "dial_end", 1)
	checkLogFields(t, logs, "listen_start", 1)
	checkLogFields(t, logs, "listen_end", 1)
	checkLogFields(t, logs, "negotiate_start", 1)
	checkLogFields(t, logs, "negotiate_end", 1)
}

func TestTCPTLSCertValidation(t *testing.T) {
	root := x509.NewCertPool()
	cert, _ := generateSelfSignedCert()
	trIface, _ := New(transport.Config{Roots: root, ClientCert: cert, Logger: zap.NewNop()})
	tr := trIface.(*Transport)
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
		close(done)
	}()
	if _, err := tr.Dial(ctx, ln.Addr().String()); err == nil {
		t.Fatalf("expected cert validation error")
	}
	<-done
}

func TestTCPTLSTransportRequiresLogger(t *testing.T) {
	cert, _ := generateSelfSignedCert()
	root := x509.NewCertPool()
	if _, err := New(transport.Config{Roots: root, ClientCert: cert}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}
