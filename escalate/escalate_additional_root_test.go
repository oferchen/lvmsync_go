//go:build root

package escalate

import (
	"os"
	"strings"
	"testing"
)

func TestEnsureRootOrReexec_SudoMissing(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	t.Setenv("PATH", "/nonexistent")
	_, err := EnsureRootOrReexec(Options{
		Geteuid:     func() int { return 1000 },
		SanitizeEnv: false,
	})
	if err == nil || !strings.Contains(err.Error(), "sudo not found") {
		t.Fatalf("expected sudo not found error, got %v", err)
	}
}

func TestDropToInvokerIfSudo_InvalidEnv(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	t.Setenv("SUDO_UID", "notnum")
	t.Setenv("SUDO_GID", "100")
	if err := DropToInvokerIfSudo(); err == nil {
		t.Fatal("expected error for invalid SUDO_UID")
	}
}

func TestEnsureRootOrReexec_AlreadyRootReal(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	reexeced, err := EnsureRootOrReexec(Options{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if reexeced {
		t.Fatal("reexeced should be false when already root")
	}
}
