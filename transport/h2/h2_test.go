package h2

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
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

func checkLogFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int) {
	entries := logs.FilterMessage(msg).All()
	if len(entries) != expected {
		t.Fatalf("expected %d %s logs, got %d", expected, msg, len(entries))
	}
	ctx := entries[0].ContextMap()
	for _, k := range []string{"address", "role", "duration_ms"} {
		if _, ok := ctx[k]; !ok {
			t.Fatalf("expected field %q in %s log", k, msg)
		}
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

func TestH2TransportTLSHandshake(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: pool, ClientCert: cert, ServerCert: cert})
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
		if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 || peerHS.ALPN != "h2" || peerHS.TLSVersion != "1.3" {
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
	if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 || peerHS.ALPN != "h2" || peerHS.TLSVersion != "1.3" {
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

	checkLogFields(t, logs, "dial_start", 1)
	checkLogFields(t, logs, "dial_end", 1)
	checkLogFields(t, logs, "listen_start", 1)
	checkLogFields(t, logs, "listen_end", 1)
	checkLogFields(t, logs, "negotiate_start", 2)
	checkLogFields(t, logs, "negotiate_end", 2)
}

func TestH2TransportTLSHandshakeError(t *testing.T) {
	serverCert, pool := generateSelfSignedCert(t)
	clientCert, _ := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	serverTrIface, err := New(transport.Config{Logger: zap.New(core), Roots: pool, ClientCert: serverCert, ServerCert: serverCert})
	if err != nil {
		t.Fatalf("server transport: %v", err)
	}
	clientTrIface, err := New(transport.Config{Logger: zap.New(core), Roots: pool, ClientCert: clientCert, ServerCert: clientCert})
	if err != nil {
		t.Fatalf("client transport: %v", err)
	}
	serverTr := serverTrIface.(*Transport)
	clientTr := clientTrIface.(*Transport)
	ctx := context.Background()
	ln, err := serverTr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	dctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := clientTr.Dial(dctx, ln.Addr().String()); err == nil {
		t.Fatalf("expected dial error")
	}

	checkLogFields(t, logs, "dial_start", 1)
	checkLogFields(t, logs, "dial_end", 1)
	checkLogFields(t, logs, "listen_start", 1)
	checkLogFields(t, logs, "listen_end", 1)
}

func TestH2TransportRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}
