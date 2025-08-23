//go:build root

package escalate

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// requireSudo checks whether the tests have the necessary privileges.
func requireSudo(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		t.Skip("sudo not installed")
	}
}

func TestEnsureRootOrReexec_ReexecSuccess(t *testing.T) {
	requireSudo(t)
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1 },
		LookPath: func(s string) (string, error) { return exec.LookPath(s) },
		ExecRunner: func(_ context.Context, _ string, _ []string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
			return nil // simulate sudo success
		},
		Stdout: io.Discard,
		Stderr: io.Discard,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reexeced {
		t.Fatalf("expected reexeced=true")
	}
}

func TestEnsureRootOrReexec_ReexecFailure(t *testing.T) {
	requireSudo(t)
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1 },
		LookPath: func(s string) (string, error) { return exec.LookPath(s) },
		ExecRunner: func(ctx context.Context, _ string, _ []string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
			return exec.CommandContext(ctx, "false").Run()
		},
		Stdout: io.Discard,
		Stderr: io.Discard,
	}, zap.NewNop())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "sudo escalation failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if reexeced {
		t.Fatalf("expected reexeced=false on error")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T", err)
	}
}
