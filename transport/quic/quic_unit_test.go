package quic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
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

func TestNewRequiresServerCert(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert}); err == nil {
		t.Fatalf("expected error when server certificate missing")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing server cert: %v", err)
	}
}

func TestNewAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure: %v", err)
	}
}

func TestNewDisables0RTT(t *testing.T) {
	trIface, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	if tr.qconf.Allow0RTT {
		t.Fatalf("expected Allow0RTT false")
	}
}

func TestNewSetsALPN(t *testing.T) {
	trIface, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	if alpn != "lvmsync" {
		t.Fatalf("unexpected alpn %q", alpn)
	}
	if len(tr.clientTLS.NextProtos) != 1 || tr.clientTLS.NextProtos[0] != alpn {
		t.Fatalf("unexpected client ALPN %v", tr.clientTLS.NextProtos)
	}
	if len(tr.serverTLS.NextProtos) != 1 || tr.serverTLS.NextProtos[0] != alpn {
		t.Fatalf("unexpected server ALPN %v", tr.serverTLS.NextProtos)
	}
}

func TestDialListenHandshake(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, ServerCert: cert})
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
	peer, err := tr.Negotiate(ctx, conn, transport.Client, hs)
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peer.ALPN != alpn || peer.TLSVersion != "1.3" {
		t.Fatalf("unexpected ALPN/TLS version %q/%q", peer.ALPN, peer.TLSVersion)
	}
	conn.Write([]byte{0})
	if err := <-srvErr; err != nil {
		t.Fatalf("server negotiate: %v", err)
	}
}

func TestDialError(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, ServerCert: cert})
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
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, ServerCert: cert})
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

type notifyingCache struct {
	tls.ClientSessionCache
	ch chan struct{}
}

func (n notifyingCache) Put(key string, cs *tls.ClientSessionState) {
	n.ClientSessionCache.Put(key, cs)
	select {
	case n.ch <- struct{}{}:
	default:
	}
}

func TestRejects0RTT(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), ClientCert: cert, ServerCert: cert, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	cache := tls.NewLRUClientSessionCache(1)
	ticketCh := make(chan struct{}, 1)
	tr.clientTLS.ClientSessionCache = notifyingCache{ClientSessionCache: cache, ch: ticketCh}

	// establish a connection to obtain a session ticket
	ln0, err := quic.ListenAddrEarly("127.0.0.1:0", tr.serverTLS, &quic.Config{Allow0RTT: true})
	if err != nil {
		t.Fatalf("listen early: %v", err)
	}
	go func() {
		conn, err := ln0.Accept(context.Background())
		if err == nil {
			<-conn.Context().Done()
		}
	}()
	conn0, err := quic.DialAddr(context.Background(), ln0.Addr().String(), tr.clientTLS, tr.qconf)
	if err != nil {
		t.Fatalf("dial initial: %v", err)
	}
	<-ticketCh
	conn0.CloseWithError(0, "")
	ln0.Close()

	// start a listener that rejects 0-RTT
	ln, err := quic.ListenAddrEarly("127.0.0.1:0", tr.serverTLS, &quic.Config{Allow0RTT: false})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		srvConn, err := ln.Accept(ctx)
		if err != nil {
			done <- err
			return
		}
		if srvConn.ConnectionState().Used0RTT {
			done <- fmt.Errorf("server accepted 0-RTT")
			return
		}
		ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel2()
		if _, err := srvConn.AcceptUniStream(ctx2); err != context.DeadlineExceeded {
			done <- fmt.Errorf("expected deadline exceeded, got %v", err)
			return
		}
		srvConn.CloseWithError(0, "")
		done <- nil
	}()

	cliConn, err := quic.DialAddrEarly(context.Background(), ln.Addr().String(), tr.clientTLS, tr.qconf)
	if err != nil {
		t.Fatalf("dial early: %v", err)
	}
	str, err := cliConn.OpenUniStream()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if _, err := str.Write([]byte("data")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := str.Close(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	ctx3, cancel3 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel3()
	if _, err := cliConn.AcceptUniStream(ctx3); !errors.Is(err, quic.Err0RTTRejected) {
		t.Fatalf("expected 0-RTT rejected, got %v", err)
	}
	if cliConn.ConnectionState().Used0RTT {
		t.Fatalf("client used 0-RTT")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	cliConn.CloseWithError(0, "")
}
