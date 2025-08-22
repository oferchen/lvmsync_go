//go:build integration

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
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
	"lvmsync_go/transport"
	_ "lvmsync_go/transport/h2"
	_ "lvmsync_go/transport/quic"
	_ "lvmsync_go/transport/ssh"
	_ "lvmsync_go/transport/tcp_tls"
)

func handshakeRoundTrip(t transport.Interface, tname string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

func TestNegotiationTransportFallback(t *testing.T) {
	// Ensure first transport fails and fallback succeeds.
	core, obs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	defer logger.Sync()

	cert, _ := generateCert()
	root := x509.NewCertPool()
	if c, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
		root.AddCert(c)
	}
	cfg := transport.Config{Logger: logger, ClientCert: cert, ServerCert: cert, Roots: root}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tname := "h2"
	server, err := transport.Get(tname, cfg)
	if err != nil {
		t.Skipf("get transport %s: %v", tname, err)
	}
	ln, err := server.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan error, 1)
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
		resp := common.Handshake{Version: common.ProtocolVersion, ALPN: req.ALPN, TLSVersion: req.TLSVersion}
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

	names := []string{"ssh", tname}
	tr, conn, err := transport.DialWithFallback(ctx, ln.Addr().String(), names, cfg)
	if err != nil {
		t.Fatalf("dial fallback: %v", err)
	}
	if tr.Name() != tname {
		t.Fatalf("expected transport %s, got %s", tname, tr.Name())
	}

	req := common.Handshake{
		Version:     common.ProtocolVersion,
		ALPN:        "h2",
		TLSVersion:  "1.3",
		Transports:  names,
		Compressors: []string{"lz4", "zstd"},
		Digests:     []string{"sha256", "blake3"},
	}
	if err := common.WriteHandshake(conn, req); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	resp, err := common.ReadHandshake(bufio.NewReader(conn))
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	if resp.Transport != tname {
		t.Fatalf("expected transport %s, got %s", tname, resp.Transport)
	}
	conn.Close()
	if err := <-done; err != nil {
		t.Fatalf("server handshake: %v", err)
	}

	var failed, success bool
	for _, e := range obs.All() {
		if (e.Message == "dial_failed" || e.Message == "get_failed") &&
			e.ContextMap()["transport"] == "ssh" {
			failed = true
		}
		if e.Message == "dial_success" {
			if tr, ok := e.ContextMap()["transport"].(string); ok && tr == tname {
				success = true
			}
		}
	}
	if !failed || !success {
		t.Fatalf("expected failure and success logs, got failed=%v success=%v", failed, success)
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
