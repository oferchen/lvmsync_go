package transport_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
	_ "lvmsync_go/transport/h2"
	_ "lvmsync_go/transport/quic"
	_ "lvmsync_go/transport/ssh"
	_ "lvmsync_go/transport/tcp_tls"
	_ "unsafe"
)

//go:linkname registry lvmsync_go/transport.registry
var registry map[string]transport.Factory

//go:linkname regMu lvmsync_go/transport.regMu
var regMu sync.RWMutex

type result struct {
	peer common.Handshake
	err  error
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

func newTransport(t *testing.T, name string) transport.Interface {
	t.Helper()
	cfg := transport.Config{Logger: zap.NewNop()}
	if name == "tcp+tls" || name == "quic" || name == "h2" {
		cert, pool := generateSelfSignedCert(t)
		cfg.ClientCert = cert
		cfg.ServerCert = cert
		cfg.Roots = pool
	}
	if name == "ssh" {
		cfg.SSHUser = "user"
		cfg.SSHPassword = "pass"
		cfg.AllowInsecure = true
	}
	tr, err := transport.Get(name, cfg)
	if err != nil {
		t.Fatalf("get transport %s: %v", name, err)
	}
	return tr
}

func runNegotiation(t *testing.T, tr transport.Interface, serverHS, clientHS common.Handshake) (result, result) {
	t.Helper()
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	srvCh := make(chan result)
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvCh <- result{err: err}
			return
		}
		peer, err := tr.Negotiate(ctx, conn, transport.Server, serverHS)
		if err != nil {
			conn.Close()
			srvCh <- result{peer: peer, err: err}
			return
		}
		<-done
		conn.Close()
		srvCh <- result{peer: peer, err: err}
	}()
	conn, err := tr.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	peer, err := tr.Negotiate(ctx, conn, transport.Client, clientHS)
	conn.Close()
	close(done)
	srvRes := <-srvCh
	return srvRes, result{peer: peer, err: err}
}

func TestHandshakeValidationMismatch(t *testing.T) {
	regMu.RLock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	regMu.RUnlock()
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tr := newTransport(t, name)
			serverHS := common.Handshake{ALPN: "lvmsync", TLSVersion: "1.3"}
			if name == "h2" {
				serverHS.ALPN = "h2"
			}
			clientHS := serverHS
			clientHS.ALPN = "other"
			clientHS.TLSVersion = "1.2"
			srv, cli := runNegotiation(t, tr, serverHS, clientHS)
			if srv.err == nil || cli.err == nil {
				t.Fatalf("expected negotiation error: server %v client %v", srv.err, cli.err)
			}
		})
	}
}
