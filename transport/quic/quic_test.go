package quic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	quic "github.com/quic-go/quic-go"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func checkLogFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int, wantErr bool) {
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
	if _, ok := ctx["error"]; wantErr && !ok {
		t.Fatalf("expected error field in %s log", msg)
	} else if !wantErr && ok {
		t.Fatalf("unexpected error field in %s log", msg)
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
		peerHS, err := tr.Negotiate(ctx, qconn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8})
		if err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 {
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
	peerHS, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8})
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 {
		t.Fatalf("unexpected peer handshake: %+v", peerHS)
	}
	qconn.Write([]byte{1})
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false)
	checkLogFields(t, logs, "dial_end", 1, false)
	checkLogFields(t, logs, "listen_start", 1, false)
	checkLogFields(t, logs, "listen_end", 1, false)
	checkLogFields(t, logs, "negotiate_start", 2, false)
	checkLogFields(t, logs, "negotiate_end", 2, false)
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
	if _, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{}); err == nil {
		t.Fatalf("expected negotiate error")
	}
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false)
	checkLogFields(t, logs, "dial_end", 1, false)
	checkLogFields(t, logs, "listen_start", 1, false)
	checkLogFields(t, logs, "listen_end", 1, false)
	checkLogFields(t, logs, "negotiate_start", 1, false)
	checkLogFields(t, logs, "negotiate_end", 1, true)
}

func TestQUICListenerAcceptContextCancel(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), ClientCert: cert, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithCancel(context.Background())
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	acceptErr := make(chan error, 1)
	go func() {
		_, err := ln.Accept()
		acceptErr <- err
	}()

	qconn, err := quic.DialAddr(context.Background(), ln.Addr().String(), tr.clientTLS, tr.qconf)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Allow Accept to block on AcceptStream.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-acceptErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("accept did not return after cancel")
	}

	qconn.CloseWithError(0, "")
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
