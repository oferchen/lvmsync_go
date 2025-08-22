package transport_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
	"github.com/oferchen/lvmsync_go/transport/testutil"

	_ "github.com/oferchen/lvmsync_go/transport/tcp_tls"
)

func TestNegotiateFailsWithoutClientCert(t *testing.T) {
	cert, root := testutil.GenerateSelfSignedCert(t)
	srv, err := transport.Get("tcp+tls", transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	ctx := context.Background()
	listenCtx, cancel := context.WithTimeout(ctx, time.Second)
	ln, err := srv.Listen(listenCtx, "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	defer cancel()
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		srvCtx, cancelSrv := context.WithTimeout(ctx, time.Second)
		_, err = srv.Negotiate(srvCtx, conn, transport.Server, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true})
		cancelSrv()
		conn.Close()
		done <- err
	}()

	cli, err := transport.Get("tcp+tls", transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, time.Second)
	conn, err := cli.Dial(dialCtx, ln.Addr().String())
	cancelDial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	negCtx, cancelNeg := context.WithTimeout(ctx, time.Second)
	if _, err := cli.Negotiate(negCtx, conn, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err == nil {
		cancelNeg()
		conn.Close()
		t.Fatalf("expected negotiate error")
	}
	cancelNeg()
	conn.Close()
	if err := <-done; err == nil {
		t.Fatalf("expected server negotiate error")
	}
}

func TestAllowInsecureBypassesTLSAndWarns(t *testing.T) {
	core, obs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	cert, _ := testutil.GenerateSelfSignedCert(t)
	srv, err := transport.Get("tcp+tls", transport.Config{Logger: logger, AllowInsecure: true, ServerCert: cert})
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	cli, err := transport.Get("tcp+tls", transport.Config{Logger: logger, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}

	ctx := context.Background()
	listenCtx, cancel := context.WithTimeout(ctx, time.Second)
	ln, err := srv.Listen(listenCtx, "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	defer cancel()
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		srvCtx, cancelSrv := context.WithTimeout(ctx, time.Second)
		_, err = srv.Negotiate(srvCtx, conn, transport.Server, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true})
		cancelSrv()
		conn.Close()
		done <- err
	}()

	dialCtx, cancelDial := context.WithTimeout(ctx, time.Second)
	conn, err := cli.Dial(dialCtx, ln.Addr().String())
	if err != nil {
		cancelDial()
		t.Fatalf("dial: %v", err)
	}
	negCtx, cancelNeg := context.WithTimeout(ctx, time.Second)
	if _, err := cli.Negotiate(negCtx, conn, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err != nil {
		cancelNeg()
		conn.Close()
		t.Fatalf("client negotiate: %v", err)
	}
	cancelNeg()
	cancelDial()
	conn.Close()
	if err := <-done; err != nil {
		t.Fatalf("server negotiate: %v", err)
	}

	if entries := obs.FilterMessage("allow_insecure_enabled").All(); len(entries) == 0 {
		t.Fatalf("expected allow_insecure warning")
	}
}
