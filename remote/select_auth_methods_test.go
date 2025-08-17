package remote

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/ssh/agent"
)

// helper to start a stub ssh agent listening on a unix socket
func startAgent(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		agent.ServeAgent(agent.NewKeyring(), conn)
	}()
	cleanup := func() { ln.Close() }
	return sock, cleanup
}

func TestSelectAuthMethodsSuccess(t *testing.T) {
	sock, cleanup := startAgent(t)
	defer cleanup()
	t.Setenv("SSH_AUTH_SOCK", sock)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	methods, err := selectAuthMethods(ctx, zap.NewNop(), "", time.Second)
	if err != nil {
		t.Fatalf("selectAuthMethods error: %v", err)
	}
	if len(methods) == 0 {
		t.Fatal("expected auth methods")
	}
}

func TestSelectAuthMethodsTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	defer cancel()
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing.sock"))
	_, err := selectAuthMethods(ctx, zap.NewNop(), "", time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestSelectAuthMethodsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing.sock"))
	_, err := selectAuthMethods(ctx, zap.NewNop(), "", time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestSelectAuthMethodsLogsAgentFailure(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "missing.sock")
	t.Setenv("SSH_AUTH_SOCK", sock)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	core, obs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	_, err := selectAuthMethods(ctx, logger, "", 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	logs := obs.All()
	if len(logs) != 1 {
		t.Fatalf("expected one log entry, got %d", len(logs))
	}
	if logs[0].Message != "ssh agent dial failed" {
		t.Fatalf("unexpected log message %q", logs[0].Message)
	}
	if logs[0].ContextMap()["socket_path"] != sock {
		t.Fatalf("expected socket_path %q, got %v", sock, logs[0].ContextMap()["socket_path"])
	}
	if _, ok := logs[0].ContextMap()["sock"]; ok {
		t.Fatalf("unexpected sock field in log context")
	}
}
