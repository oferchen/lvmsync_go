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

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

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

func TestNewRequiresClientCert(t *testing.T) {
	_, roots := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots}); err == nil {
		t.Fatalf("expected error when client certificate missing")
	}
}

func TestNewAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure: %v", err)
	}
}

func TestDialListenHandshake(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	hs := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		qconn := conn.(*Conn)
		if _, err = tr.Negotiate(ctx, qconn, transport.Server, hs); err == nil {
			buf := make([]byte, 1)
			qconn.Read(buf)
		}
		qconn.Close()
		srvErr <- err
	}()
	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := tr.Negotiate(ctx, conn, transport.Client, hs); err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	conn.Write([]byte{0})
	if err := <-srvErr; err != nil {
		t.Fatalf("server negotiate: %v", err)
	}
}

func TestDialError(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := tr.Dial(ctx, "127.0.0.1:1"); err == nil {
		t.Fatalf("expected dial error")
	}
}

func TestListenError(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := tr.Listen(ctx, "127.0.0.1:99999"); err == nil {
		t.Fatalf("expected listen error")
	}
}

func TestNegotiateMismatch(t *testing.T) {
	tr := &Transport{logger: zap.NewNop()}
	c1, c2 := net.Pipe()
	hs1 := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
	hs2 := common.Handshake{CDCMin: 64, CDCAvg: 256, CDCMax: 256, CRC32C: true}
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() {
		_, err := tr.Negotiate(ctx, c1, transport.Client, hs1)
		errCh <- err
	}()
	if _, err := tr.Negotiate(ctx, c2, transport.Server, hs2); err == nil {
		t.Fatalf("expected server negotiate error")
	}
	c2.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected client negotiate error")
	}
	c1.Close()
}
