package escalate

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestEnsureRootOrReexec_NoReexecWhenRoot verifies that running as root
// does not attempt to re-exec through sudo.
func TestEnsureRootOrReexec_NoReexecWhenRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		t.Skip("sudo not installed")
	}
	lookCalled := false
	runCalled := false
	opts := Options{
		LookPath: func(s string) (string, error) {
			lookCalled = true
			return exec.LookPath(s)
		},
		ExecRunner: func(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) error {
			runCalled = true
			return nil
		},
	}
	reexeced, err := EnsureRootOrReexec(opts, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if reexeced {
		t.Fatalf("expected reexeced false, got true")
	}
	if lookCalled {
		t.Fatalf("LookPath should not be called for root")
	}
	if runCalled {
		t.Fatalf("ExecRunner should not be called for root")
	}
}

// TestEnsureRootOrReexec_InvalidCommand ensures errors are surfaced when the
// escalation command fails.
func TestEnsureRootOrReexec_InvalidCommand(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if _, err := os.Stat("/bin/false"); err != nil {
		t.Skip("/bin/false not present")
	}
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1 },
		LookPath: func(string) (string, error) { return "/bin/false", nil },
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "sudo escalation failed") {
		t.Fatalf("expected sudo escalation error, got %v", err)
	}
	if reexeced {
		t.Fatalf("expected reexeced false, got true")
	}
}

// TestEnsureRootOrReexec_PipedSleep verifies that piped commands do not hang
// and respect timeouts.
func TestEnsureRootOrReexec_PipedSleep(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	ctxTimeout := 100 * time.Millisecond
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1 },
		LookPath: func(string) (string, error) { return "/bin/sh", nil },
		ExecRunner: func(ctx context.Context, _ string, _ []string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
			return exec.CommandContext(ctx, "sh", "-c", "sleep 5 | true").Run()
		},
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		Timeout: ctxTimeout,
	}, zap.NewNop())
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if reexeced {
		t.Fatalf("expected reexeced false, got true")
	}
}

// TestEnsureRootOrReexec_SleepTimeout verifies that long-running commands are
// killed when exceeding the timeout.
func TestEnsureRootOrReexec_SleepTimeout(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	ctxTimeout := 100 * time.Millisecond
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1 },
		LookPath: func(string) (string, error) { return "/bin/sh", nil },
		ExecRunner: func(ctx context.Context, _ string, _ []string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
			return exec.CommandContext(ctx, "sh", "-c", "sleep 5").Run()
		},
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		Timeout: ctxTimeout,
	}, zap.NewNop())
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if reexeced {
		t.Fatalf("expected reexeced false, got true")
	}
}
