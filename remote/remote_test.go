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
	os.Unsetenv("SSH_AUTH_SOCK")
	_, err := NewSSHManager("root", "", time.Second, "", false)
	if err == nil {
		t.Fatal("expected error when SSH_AUTH_SOCK not set")
	}
}

func TestNewSSHClientNoAuth(t *testing.T) {
	oldSock := os.Getenv("SSH_AUTH_SOCK")
	os.Unsetenv("SSH_AUTH_SOCK")
	defer func() {
		if oldSock != "" {
			os.Setenv("SSH_AUTH_SOCK", oldSock)
		}
	}()

	_, err := NewSSHClient("localhost", "root", "", 22, "", false, time.Second, time.Second, 0)
	if err == nil || !strings.Contains(err.Error(), "no SSH authentication methods configured") {
		t.Fatalf("expected error for missing auth methods, got %v", err)
	}
}
