package remote

import (
	"os"
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
