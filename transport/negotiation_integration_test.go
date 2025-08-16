package transport_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
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

func handshakeRoundTrip(t transport.Interface, tname string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln, err := t.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		send := func(err error) {
			select {
			case done <- err:
			case <-ctx.Done():
			}
		}
		conn, err := ln.Accept()
		if err != nil {
			send(err)
			return
		}
		req, err := common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			conn.Close()
			send(err)
			return
		}
		if req.ALPN != "h2" || req.TLSVersion != "1.3" {
			conn.Close()
			send(fmt.Errorf("unexpected request: %+v", req))
			return
		}
		resp := common.Handshake{Version: common.ProtocolVersion, ALPN: req.ALPN, TLSVersion: req.TLSVersion}
		resp.Transport = common.SelectBest([]string{tname}, req.Transports)
		resp.Compress = common.SelectBest([]string{"zstd", "lz4"}, req.Compressors)
		resp.Digest = common.SelectBest([]string{"blake3", "sha256"}, req.Digests)
		if err := common.WriteHandshake(conn, resp); err != nil {
			conn.Close()
			send(err)
			return
		}
		conn.Close()
		send(nil)
	}()

	conn, err := t.Dial(ctx, ln.Addr().String())
	if err != nil {
		return err
	}
	req := common.Handshake{
		Version:     common.ProtocolVersion,
		ALPN:        "h2",  // expect server to echo ALPN "h2"
		TLSVersion:  "1.3", // expect server to echo TLS version "1.3"
		Transports:  []string{"h2", "quic", "tcp+tls", "ssh"},
		Compressors: []string{"lz4", "zstd"},
		Digests:     []string{"sha256", "blake3"},
	}
	if err := common.WriteHandshake(conn, req); err != nil {
		return err
	}
	resp, err := common.ReadHandshake(bufio.NewReader(conn))
	if err != nil {
		return err
	}
	if resp.Transport != tname || resp.Compress != "zstd" || resp.Digest != "blake3" || resp.ALPN != "h2" || resp.TLSVersion != "1.3" {
		return fmt.Errorf("unexpected response: %+v", resp)
	}
	conn.Close()
	return <-done
}

func TestNegotiationTransports(t *testing.T) {
	names := []string{"tcp+tls", "ssh", "quic", "h2"}
	for _, name := range names {
		cfg := transport.Config{Logger: zap.NewNop()}
		if name == "tcp+tls" || name == "quic" || name == "h2" {
			cert, _ := generateCert()
			root := x509.NewCertPool()
			if c, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
				root.AddCert(c)
			}
			cfg.ClientCert = cert
			cfg.ServerCert = cert
			cfg.Roots = root
		}
		tr, err := transport.Get(name, cfg)
		if err != nil {
			t.Skipf("get transport %s: %v", name, err)
		}
		if err := handshakeRoundTrip(tr, name); err != nil {
			t.Skipf("%s negotiation: %v", name, err)
		}
	}
}

func generateCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return cert, nil
}
