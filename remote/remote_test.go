package remote

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewSSHManagerInvalidKey(t *testing.T) {
	_, err := NewSSHManager("root", "no_such_key", time.Second, "", false)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestNewSSHManagerNoAgent(t *testing.T) {
	if err := os.Unsetenv("SSH_AUTH_SOCK"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	_, err := NewSSHManager("root", "", time.Second, "", false)
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

	_, err := NewSSHClient("localhost", "root", "", 22, "", false, time.Second, time.Second, 0)
	if err == nil || !strings.Contains(err.Error(), "no SSH authentication methods configured") {
		t.Fatalf("expected error for missing auth methods, got %v", err)
	}
}
