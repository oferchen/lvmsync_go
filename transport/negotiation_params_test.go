package transport_test

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
	_ "lvmsync_go/transport/h2"
	_ "lvmsync_go/transport/quic"
	_ "lvmsync_go/transport/ssh"
	_ "lvmsync_go/transport/tcp_tls"
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
		cfg.SSHUser = "test"
		cfg.SSHPassword = "pass"
	}
	tr, err := transport.Get(name, cfg)
	if err != nil {
		t.Fatalf("get transport %s: %v", name, err)
	}
	return tr
}

type result struct {
	peer common.Handshake
	err  error
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

func TestTransportNegotiationMatrix(t *testing.T) {
	transports := []string{"ssh", "tcp+tls", "h2", "quic"}
	for _, name := range transports {
		t.Run(name, func(t *testing.T) {
			tr := newTransport(t, name)
			base := common.Handshake{
				DedupMode:     "fixed",
				CDCMin:        64,
				CDCAvg:        128,
				CDCMax:        256,
				Compress:      "zstd",
				CompressLevel: 1,
				ODirect:       true,
				ResumeToken:   "tok",
				MaxInFlight:   8,
				Endianness:    common.NativeEndianness(),
			}
			modes := []string{"fixed", "cdc", "hybrid"}
			for _, m := range modes {
				serverHS := base
				clientHS := base
				serverHS.DedupMode = m
				clientHS.DedupMode = m
				srv, cli := runNegotiation(t, tr, serverHS, clientHS)
				if srv.err != nil || cli.err != nil {
					t.Fatalf("expected success: server %v client %v", srv.err, cli.err)
				}
				if cli.peer.DedupMode != m || cli.peer.Compress != "zstd" || cli.peer.ODirect != true ||
					cli.peer.CDCMin != base.CDCMin || cli.peer.CDCAvg != base.CDCAvg || cli.peer.CDCMax != base.CDCMax {
					t.Fatalf("unexpected peer handshake: %+v", cli.peer)
				}
			}
			// mismatch cases
			// dedup mode
			serverHS := base
			clientHS := base
			clientHS.DedupMode = "cdc"
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected dedup mismatch error")
			}
			// cdc min
			serverHS = base
			serverHS.DedupMode = "cdc"
			clientHS = serverHS
			clientHS.CDCMin = 128
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected cdc min mismatch error")
			}
			// cdc avg
			serverHS = base
			serverHS.DedupMode = "cdc"
			clientHS = serverHS
			clientHS.CDCAvg = 256
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected cdc avg mismatch error")
			}
			// cdc max
			serverHS = base
			serverHS.DedupMode = "cdc"
			clientHS = serverHS
			clientHS.CDCMax = 512
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected cdc max mismatch error")
			}
			// compression algorithm
			serverHS = base
			clientHS = base
			clientHS.Compress = "lz4"
			clientHS.CompressLevel = 0
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected compression mismatch error")
			}
			// compression level
			serverHS = base
			clientHS = base
			clientHS.CompressLevel = 2
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected compression level mismatch error")
			}
			// O_DIRECT
			serverHS = base
			clientHS = base
			clientHS.ODirect = false
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected o_direct mismatch error")
			}
			// resume token
			serverHS = base
			clientHS = base
			clientHS.ResumeToken = "other"
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected resume token mismatch error")
			}
			// max in-flight
			serverHS = base
			clientHS = base
			clientHS.MaxInFlight = 4
			if srv, cli := runNegotiation(t, tr, serverHS, clientHS); srv.err == nil || cli.err == nil {
				t.Fatalf("expected max in-flight mismatch error")
			}
		})
	}
}
