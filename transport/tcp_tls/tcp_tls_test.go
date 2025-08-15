package tcp_tls

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func checkLogFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int, wantErr bool) {
	entries := logs.FilterMessage(msg).All()
	if len(entries) != expected {
		t.Fatalf("expected %d %s logs, got %d", expected, msg, len(entries))
	}
	if expected == 0 {
		return
	}
	ctx := entries[0].ContextMap()
	for _, k := range []string{"address", "role", "duration_ms"} {
		if _, ok := ctx[k]; !ok {
			t.Fatalf("expected field %q in %s log", k, msg)
		}
	}
	if _, ok := ctx["error"]; wantErr && !ok {
		t.Fatalf("expected error field in %s log", msg)
	} else if !wantErr && ok {
		t.Fatalf("unexpected error field in %s log", msg)
	}
}

func TestTCPTLSTransportHandshake(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
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
		peerHS, err := tr.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8})
		if err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 {
			t.Errorf("unexpected peer handshake: %+v", peerHS)
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
	peerHS, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8})
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 {
		t.Fatalf("unexpected peer handshake: %+v", peerHS)
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

	checkLogFields(t, logs, "dial_start", 1, false)
	checkLogFields(t, logs, "dial_end", 1, false)
	checkLogFields(t, logs, "listen_start", 1, false)
	checkLogFields(t, logs, "listen_end", 1, false)
	checkLogFields(t, logs, "negotiate_start", 2, false)
	checkLogFields(t, logs, "negotiate_end", 2, false)
}

func TestTCPTLSTransportHandshakeError(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
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

	checkLogFields(t, logs, "dial_start", 1, false)
	checkLogFields(t, logs, "dial_end", 1, false)
	checkLogFields(t, logs, "listen_start", 1, false)
	checkLogFields(t, logs, "listen_end", 1, false)
	checkLogFields(t, logs, "negotiate_start", 1, false)
	checkLogFields(t, logs, "negotiate_end", 1, true)
}

func TestTCPTLSNegotiateInvalidRole(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	if _, err := tr.Negotiate(ctx, c1, transport.Role(99), common.Handshake{}); err == nil {
		t.Fatalf("expected error for invalid role")
	}
}

func TestTCPTLSCertValidation(t *testing.T) {
	root := x509.NewCertPool()
	cert, _ := generateSelfSignedCert(t)
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

func TestTCPTLSDialContextCancel(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(io.Discard, conn)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	if _, err := tr.Dial(ctx, ln.Addr().String()); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestTCPTLSTransportRequiresLogger(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Roots: root, ClientCert: cert}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestTCPTLSTransportRequiresRootsOrAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop()}); err == nil {
		t.Fatalf("expected error when roots are nil without AllowInsecure")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing roots: %v", err)
	}
}

func TestTCPTLSTransportRequiresClientCert(t *testing.T) {
	root := x509.NewCertPool()
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: root}); err == nil {
		t.Fatalf("expected error when client cert is nil")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing client cert: %v", err)
	}
}

func generateSelfSignedCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return cert, pool
}
