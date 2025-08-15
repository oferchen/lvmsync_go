package quic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
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

func checkLogFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int, wantErr bool, level zapcore.Level) {
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
	if entries[0].Level != level {
		t.Fatalf("expected level %v for %s log, got %v", level, msg, entries[0].Level)
	}
}

func TestQUICTransportHandshake(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), ClientCert: cert, AllowInsecure: true})
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
		peerHS, err := tr.Negotiate(ctx, qconn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256})
		if err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 || peerHS.CDCMin != 64 || peerHS.CDCAvg != 128 || peerHS.CDCMax != 256 {
			t.Errorf("unexpected peer handshake: %+v", peerHS)
		}
		buf := make([]byte, 1)
		qconn.Read(buf)
		qconn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	peerHS, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256})
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 || peerHS.CDCMin != 64 || peerHS.CDCAvg != 128 || peerHS.CDCMax != 256 {
		t.Fatalf("unexpected peer handshake: %+v", peerHS)
	}
	qconn.Write([]byte{1})
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
}

func TestQUICTransportHandshakeError(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), ClientCert: cert, AllowInsecure: true})
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
	if _, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256}); err == nil {
		t.Fatalf("expected negotiate error")
	}
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 1, true, zapcore.ErrorLevel)
}

func TestQUICTransportHandshakeCDCMismatch(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), ClientCert: cert, AllowInsecure: true})
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
		qconn := conn.(*Conn)
		if _, err := tr.Negotiate(ctx, qconn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256}); err == nil {
			t.Errorf("expected server negotiate error")
		}
		qconn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	if _, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 256, CDCMax: 256}); err == nil {
		t.Fatalf("expected client negotiate error")
	}
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, true, zapcore.ErrorLevel)
}

func TestQUICTransportRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestQUICTransportRequiresRootsOrAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop()}); err == nil {
		t.Fatalf("expected error when roots are nil without AllowInsecure")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing roots: %v", err)
	}
}

func TestQUICTransportRequiresClientCert(t *testing.T) {
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
