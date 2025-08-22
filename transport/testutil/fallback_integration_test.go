//go:build integration

package testutil

import (
	"context"
	"io"
	"reflect"
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

func TestNegotiationClearsDeadline(t *testing.T) {
	names := []string{"ssh", "tcp+tls", "h2", "quic"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tr := NewTransport(t, name)
			hs := common.Handshake{ALPN: "lvmsync", TLSVersion: "1.3", BlockSize: 4096}
			if name == "h2" {
				hs.ALPN = "h2"
			}
			ctx := context.Background()
			ln, err := tr.Listen(ctx, "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer ln.Close()
			addr := ln.Addr().String()
			srvCh := make(chan error)
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					srvCh <- err
					return
				}
				defer conn.Close()
				sctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
				if _, err := tr.Negotiate(sctx, conn, transport.Server, hs); err != nil {
					srvCh <- err
					return
				}
				buf := make([]byte, 1)
				_, err = io.ReadFull(conn, buf)
				srvCh <- err
			}()
			cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()
			conn, err := tr.Dial(cctx, addr)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			if _, err := tr.Negotiate(cctx, conn, transport.Client, hs); err != nil {
				t.Fatalf("negotiate: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
			if _, err := conn.Write([]byte{1}); err != nil {
				t.Fatalf("write after deadline: %v", err)
			}
			if err := conn.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			if err := <-srvCh; err != nil {
				t.Fatalf("server: %v", err)
			}
		})
	}
}

func TestDialWithFallbackOrderIntegration(t *testing.T) {
	cert, pool := GenerateSelfSignedCert(t)
	core, obs := observer.New(zap.InfoLevel)
	cfg := transport.Config{
		Logger:        zap.New(core),
		ClientCert:    cert,
		ServerCert:    cert,
		Roots:         pool,
		SSHUser:       "user",
		SSHPassword:   "pass",
		AllowInsecure: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	names := []string{"quic", "h2", "tcp+tls", "ssh"}
	if _, _, err := transport.DialWithFallback(ctx, "127.0.0.1:9", names, cfg); err == nil {
		t.Fatalf("expected error")
	}
	var seq []string
	for _, entry := range obs.All() {
		if entry.Message == "dial_attempt" {
			if n, ok := entry.ContextMap()["transport"].(string); ok {
				seq = append(seq, n)
			}
		}
	}
	if !reflect.DeepEqual(seq, names) {
		t.Fatalf("attempt sequence %v, want %v", seq, names)
	}
}
