package testutil

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

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
)

func GenerateSelfSignedCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
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

func NewTransport(t *testing.T, name string) transport.Interface {
	t.Helper()
	cfg := transport.Config{Logger: zap.NewNop()}
	if name == "tcp+tls" || name == "quic" || name == "h2" {
		cert, pool := GenerateSelfSignedCert(t)
		cfg.ClientCert = cert
		cfg.ServerCert = cert
		cfg.Roots = pool
	}
	if name == "ssh" {
		cfg.SSHUser = "test"
		cfg.SSHPassword = "pass"
		cfg.AllowInsecure = true
	}
	tr, err := transport.Get(name, cfg)
	if err != nil {
		t.Fatalf("get transport %s: %v", name, err)
	}
	if tr == nil {
		t.Fatalf("get transport %s: nil", name)
	}
	return tr
}

type Result struct {
	Peer common.Handshake
	Err  error
}

func RunNegotiation(t *testing.T, tr transport.Interface, serverHS, clientHS common.Handshake) (Result, Result) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	srvCh := make(chan Result)
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvCh <- Result{Err: err}
			return
		}
		peer, err := tr.Negotiate(ctx, conn, transport.Server, serverHS)
		if err != nil {
			conn.Close()
			srvCh <- Result{Peer: peer, Err: err}
			return
		}
		<-done
		conn.Close()
		srvCh <- Result{Peer: peer, Err: err}
	}()
	conn, err := tr.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	peer, err := tr.Negotiate(ctx, conn, transport.Client, clientHS)
	conn.Close()
	close(done)
	srvRes := <-srvCh
	return srvRes, Result{Peer: peer, Err: err}
}

func RunNegotiationWithDelay(t *testing.T, tr transport.Interface, serverHS, clientHS common.Handshake, delay, timeout time.Duration) (Result, Result) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	srvCh := make(chan Result)
	go func() {
		time.Sleep(delay)
		conn, err := ln.Accept()
		if err != nil {
			srvCh <- Result{Err: err}
			return
		}
		peer, err := tr.Negotiate(ctx, conn, transport.Server, serverHS)
		conn.Close()
		srvCh <- Result{Peer: peer, Err: err}
	}()
	conn, err := tr.Dial(ctx, addr)
	if err != nil {
		srvRes := <-srvCh
		return srvRes, Result{Err: err}
	}
	peer, err := tr.Negotiate(ctx, conn, transport.Client, clientHS)
	conn.Close()
	srvRes := <-srvCh
	return srvRes, Result{Peer: peer, Err: err}
}
