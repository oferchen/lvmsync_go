package escalate

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"go.uber.org/zap"
)

type stubSyscall struct {
	gids []int
	uid  int
	gid  int
}

func (s *stubSyscall) Setgroups(gids []int) error {
	s.gids = append([]int(nil), gids...)
	return nil
}
func (s *stubSyscall) Setresgid(r, e, gid int) error {
	s.gid = r
	return nil
}
func (s *stubSyscall) Setresuid(r, e, uid int) error {
	s.uid = r
	return nil
}

// TestEnsureRootOrReexec_SimulatedRootNonRoot exercises both success and error paths
// while simulating execution as root and as a regular user. Tests are skipped
// entirely when not running as root.
func TestEnsureRootOrReexec_SimulatedRootNonRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	// Success when already running as root.
	reexeced, err := EnsureRootOrReexec(Options{}, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reexeced {
		t.Fatalf("reexeced should be false when already root")
	}

	// Simulate non-root and successful sudo re-exec.
	var ran bool
	reexeced, err = EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1 },
		LookPath: func(string) (string, error) { return "/usr/bin/sudo", nil },
		ExecRunner: func(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) error {
			ran = true
			return nil
		},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reexeced || !ran {
		t.Fatalf("expected reexeced=true and runner invoked, got %v and ran=%v", reexeced, ran)
	}

	// Simulate non-root with sudo missing to trigger error path.
	_, err = EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1 },
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
	}, zap.NewNop())
	if err == nil {
		t.Fatal("expected error when sudo missing")
	}
}

// TestDropToInvokerIfSudo_SimulatedRootNonRoot tests dropping privileges when
// launched via sudo and error handling for bad environments. Tests are skipped
// when not running as root.
func TestDropToInvokerIfSudo_SimulatedRootNonRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	// Success path: valid sudo environment.
	t.Setenv("SUDO_UID", "1")
	t.Setenv("SUDO_GID", "1")
	sys := &stubSyscall{}
	if err := DropToInvokerIfSudo(Options{Sys: sys}, zap.NewNop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sys.gids) != 1 || sys.gids[0] != 1 || sys.uid != 1 || sys.gid != 1 {
		t.Fatalf("unexpected syscall arguments: %+v", sys)
	}

	// Non-root (no sudo) should be a no-op.
	t.Setenv("SUDO_UID", "")
	t.Setenv("SUDO_GID", "")
	if err := DropToInvokerIfSudo(Options{}, zap.NewNop()); err != nil {
		t.Fatalf("expected nil when not using sudo: %v", err)
	}

	// Error path: invalid environment variables.
	t.Setenv("SUDO_UID", "notnum")
	t.Setenv("SUDO_GID", "1")
	if err := DropToInvokerIfSudo(Options{}, zap.NewNop()); err == nil {
		t.Fatal("expected error for invalid SUDO_UID")
	}
}
