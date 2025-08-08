package remote

import (
	"os"
	"strings"
	"testing"
	"time"

	remotetest "lvmsync_go/remote/testutil"
)

func TestNewSSHManagerInvalidKey(t *testing.T) {
	knownHosts := remotetest.CreateEmptyKnownHosts(t)
	_, err := NewSSHManager("root", "no_such_key", time.Second, knownHosts)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestNewSSHManagerNoAgent(t *testing.T) {
	if err := os.Unsetenv("SSH_AUTH_SOCK"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	knownHosts := remotetest.CreateEmptyKnownHosts(t)
	_, err := NewSSHManager("root", "", time.Second, knownHosts)
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
	_, err := NewSSHClient("localhost", "root", "", 22, knownHosts, true, time.Second, time.Second, 0)
	if err == nil || !strings.Contains(err.Error(), "no SSH authentication methods configured") {
		t.Fatalf("expected error for missing auth methods, got %v", err)
	}
}
