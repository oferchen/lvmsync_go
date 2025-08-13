package remote

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"lvmsync_go/remote/testutil"
)

func TestSSHManagerGetClientReuse(t *testing.T) {
	server := testutil.NewMockSSHServer(t, func(string) int { return 0 })
	defer server.Close()
	knownHosts := testutil.CreateKnownHostsFile(t, server)
	keyPath := testutil.CreateTempKey(t)

	initCtx, initCancel := context.WithTimeout(context.Background(), time.Second)
	defer initCancel()
	mgr, err := NewSSHManager(initCtx, "user", keyPath, time.Second, knownHosts, zaptest.NewLogger(t))
	if err != nil {
		t.Fatalf("NewSSHManager: %v", err)
	}

	host, portStr, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c1, err := mgr.GetClient(ctx, host, port)
	if err != nil {
		t.Fatalf("GetClient first: %v", err)
	}
	c2, err := mgr.GetClient(ctx, host, port)
	if err != nil {
		t.Fatalf("GetClient second: %v", err)
	}
	if c1 != c2 {
		t.Fatal("expected connection reuse")
	}
	if server.ConnectionCount() != 1 {
		t.Fatalf("expected 1 connection, got %d", server.ConnectionCount())
	}
	if err := mgr.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
}

func TestSSHManagerGetClientRefresh(t *testing.T) {
	server := testutil.NewMockSSHServer(t, func(string) int { return 0 })
	defer server.Close()
	knownHosts := testutil.CreateKnownHostsFile(t, server)
	keyPath := testutil.CreateTempKey(t)

	initCtx, initCancel := context.WithTimeout(context.Background(), time.Second)
	defer initCancel()
	mgr, err := NewSSHManager(initCtx, "user", keyPath, time.Second, knownHosts, zaptest.NewLogger(t))
	if err != nil {
		t.Fatalf("NewSSHManager: %v", err)
	}

	host, portStr, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c1, err := mgr.GetClient(ctx, host, port)
	if err != nil {
		t.Fatalf("GetClient first: %v", err)
	}
	if server.ConnectionCount() != 1 {
		t.Fatalf("expected 1 connection, got %d", server.ConnectionCount())
	}

	c1.Close()
	time.Sleep(50 * time.Millisecond)

	c2, err := mgr.GetClient(ctx, host, port)
	if err != nil {
		t.Fatalf("GetClient second: %v", err)
	}
	if c1 == c2 {
		t.Fatal("expected new connection after close")
	}
	if server.ConnectionCount() != 2 {
		t.Fatalf("expected 2 connections, got %d", server.ConnectionCount())
	}
	if err := mgr.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
}

func TestSSHAgentAuthTimeout(t *testing.T) {
	tmpSock := filepath.Join(t.TempDir(), "agent.sock")
	os.Setenv("SSH_AUTH_SOCK", tmpSock)
	defer os.Unsetenv("SSH_AUTH_SOCK")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := sshAgentAuth(ctx, 50*time.Millisecond, zaptest.NewLogger(t))
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestSSHAgentAuthSuccess(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		agent.ServeAgent(agent.NewKeyring(), conn)
	}()

	os.Setenv("SSH_AUTH_SOCK", sock)
	defer os.Unsetenv("SSH_AUTH_SOCK")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	auth, err := sshAgentAuth(ctx, time.Second, zaptest.NewLogger(t))
	if err != nil {
		t.Fatalf("sshAgentAuth: %v", err)
	}
	if auth == nil {
		t.Fatal("expected auth method")
	}
}

func TestLoadPrivateKeyZeroesSlice(t *testing.T) {
	keyPath := testutil.CreateTempKey(t)
	var captured []byte
	parsePrivateKey = func(data []byte) (ssh.Signer, error) {
		captured = data
		return ssh.ParsePrivateKey(data)
	}
	t.Cleanup(func() { parsePrivateKey = ssh.ParsePrivateKey })

	signer, err := loadPrivateKey(keyPath)
	if err != nil {
		t.Fatalf("loadPrivateKey: %v", err)
	}
	if signer == nil {
		t.Fatal("expected signer")
	}
	for _, b := range captured {
		if b != 0 {
			t.Fatal("key data not zeroed")
		}
	}
}

func TestLoadPrivateKeyPermissions(t *testing.T) {
	goodKey := testutil.CreateTempKey(t)
	openKey := testutil.CreateTempKey(t)
	if err := os.Chmod(openKey, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	missingKey := filepath.Join(t.TempDir(), "missing")

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "correct", path: goodKey, wantErr: false},
		{name: "too_open", path: openKey, wantErr: true},
		{name: "missing", path: missingKey, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadPrivateKey(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s", tt.name)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %s: %v", tt.name, err)
				}
			}
		})
	}
}
