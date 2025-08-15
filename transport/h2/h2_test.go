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
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func checkLogFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int, level zapcore.Level) {
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
	if entries[0].Level != level {
		t.Fatalf("expected level %v for %s log, got %v", level, msg, entries[0].Level)
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
		peerHS, err := tr.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256})
		if err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 || peerHS.ALPN != "h2" || peerHS.TLSVersion != "1.3" ||
			peerHS.CDCMin != 64 || peerHS.CDCAvg != 128 || peerHS.CDCMax != 256 {
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
	peerHS, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256})
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 || peerHS.ALPN != "h2" || peerHS.TLSVersion != "1.3" ||
		peerHS.CDCMin != 64 || peerHS.CDCAvg != 128 || peerHS.CDCMax != 256 {
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

	checkLogFields(t, logs, "dial_start", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, zapcore.InfoLevel)
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

	checkLogFields(t, logs, "dial_start", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, zapcore.ErrorLevel)
	checkLogFields(t, logs, "listen_start", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, zapcore.InfoLevel)
}

func TestH2TransportTLSCDCMismatch(t *testing.T) {
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
			close(done)
			return
		}
		if _, err := tr.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256}); err == nil {
			t.Errorf("expected server negotiate error")
		}
		conn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 256, CDCMax: 256}); err == nil {
		t.Fatalf("expected client negotiate error")
	}
	conn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, zapcore.ErrorLevel)

	entries := logs.FilterMessage("negotiate_end").All()
	for _, e := range entries {
		if _, ok := e.ContextMap()["error"]; !ok {
			t.Fatalf("expected error field in negotiate_end log")
		}
	}
}

func TestH2TransportRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}
