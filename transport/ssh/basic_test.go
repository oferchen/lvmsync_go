package ssh

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func TestNewRequiresUser(t *testing.T) {
	if _, err := New(context.Background(), transport.Config{Logger: zap.NewNop(), SSHPassword: "p"}); err == nil {
		t.Fatalf("expected error for missing user")
	}
}

func TestNewRequiresLogger(t *testing.T) {
	cfg := transport.Config{SSHUser: "u", SSHPassword: "p", AllowInsecure: true}
	if _, err := New(context.Background(), cfg); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestDialAndNegotiate(t *testing.T) {
	cfg := transport.Config{Logger: zap.NewNop(), SSHUser: "u", SSHPassword: "p", AllowInsecure: true}
	trIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_, err = tr.Negotiate(ctx, conn, transport.Server, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true})
		done <- err
	}()
	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("server negotiate: %v", err)
	}
}
