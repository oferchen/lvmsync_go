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
	_ "lvmsync_go/transport/ssh"
	_ "lvmsync_go/transport/tcp_tls"
)

func handshakeRoundTrip(t transport.Interface, tname string) error {
	ctx := context.Background()
	ln, err := t.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()

	done := make(chan error)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		req, err := common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			conn.Close()
			done <- err
			return
		}
		resp := common.Handshake{Version: common.ProtocolVersion}
		resp.Transport = common.SelectBest([]string{tname}, req.Transports)
		resp.Compress = common.SelectBest([]string{"zstd", "lz4"}, req.Compressors)
		resp.Digest = common.SelectBest([]string{"blake3", "sha256"}, req.Digests)
		if err := common.WriteHandshake(conn, resp); err != nil {
			conn.Close()
			done <- err
			return
		}
		conn.Close()
		done <- nil
	}()

	conn, err := t.Dial(ctx, ln.Addr().String())
	if err != nil {
		return err
	}
	req := common.Handshake{
		Version:     common.ProtocolVersion,
		Transports:  []string{"tcp+tls", "ssh"},
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
	if resp.Transport == "" || resp.Compress == "" || resp.Digest == "" {
		return fmt.Errorf("incomplete response: %+v", resp)
	}
	conn.Close()
	return <-done
}

func TestNegotiationTCPAndSSH(t *testing.T) {
	names := []string{"tcp+tls", "ssh"}
	for _, name := range names {
		cfg := transport.Config{Logger: zap.NewNop()}
		if name == "tcp+tls" {
			cert, _ := generateCert()
			root := x509.NewCertPool()
			if c, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
				root.AddCert(c)
			}
			cfg.ClientCert = cert
			cfg.Roots = root
		}
		tr, err := transport.Get(name, cfg)
		if err != nil {
			t.Fatalf("get transport %s: %v", name, err)
		}
		if err := handshakeRoundTrip(tr, name); err != nil {
			t.Fatalf("%s negotiation: %v", name, err)
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
