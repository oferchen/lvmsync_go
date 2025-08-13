package remote

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	remotetest "lvmsync_go/remote/testutil"
)

func TestNewSSHManagerInvalidKey(t *testing.T) {
	knownHosts := remotetest.CreateEmptyKnownHosts(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := NewSSHManager(ctx, "root", "no_such_key", time.Second, knownHosts, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestNewSSHManagerNoAgent(t *testing.T) {
	if err := os.Unsetenv("SSH_AUTH_SOCK"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	knownHosts := remotetest.CreateEmptyKnownHosts(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := NewSSHManager(ctx, "root", "", time.Second, knownHosts, zap.NewNop())
	if err == nil {
		t.Fatal("expected error when SSH_AUTH_SOCK not set")
	}
}

func TestNewSSHClientNoAuth(t *testing.T) {
	oldSock := os.Getenv("SSH_AUTH_SOCK")
	if err := os.Unsetenv("SSH_AUTH_SOCK"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	defer func() {
		if oldSock != "" {
			if err := os.Setenv("SSH_AUTH_SOCK", oldSock); err != nil {
				t.Fatalf("Setenv: %v", err)
			}
		}
	}()

	knownHosts := remotetest.CreateEmptyKnownHosts(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := NewSSHClient(ctx, "localhost", "root", "", 22, knownHosts, true, time.Second, time.Second, 0, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "no SSH authentication methods configured") {
		t.Fatalf("expected error for missing auth methods, got %v", err)
	}
}

func TestDialWithRetryContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	config := &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Millisecond,
	}
	cancel()
	_, err := dialWithRetry(ctx, zap.NewNop(), "127.0.0.1:22", config, "127.0.0.1", 22, 3)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}
