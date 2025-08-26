//go:build root

package privilege_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/exitcode"
	"lvmsync_go/internal/privilege"
)

func TestEnsureSudoTrue(t *testing.T) {
	requireSudo(t)
	privilege.HasCaps = func() bool { return false }
	defer func() { privilege.HasCaps = privilege.RealHasCaps }()
	esc, err := privilege.New(context.Background(), zap.NewNop())
	if err != nil {
		t.Fatalf("privilege.New: %v", err)
	}
	if err := esc.Ensure(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureSudoErrorsExitCode(t *testing.T) {
	requireRoot(t)
	tests := []struct {
		name   string
		runner *privilege.Runner
	}{
		{
			name:   "missing_sudo",
			runner: &privilege.Runner{LookPath: fakeLookPath(errors.New("not found"))},
		},
		{
			name: "denied",
			runner: &privilege.Runner{
				Cmd:      cmdFunc(func(_ context.Context, name string, args ...string) *exec.Cmd { return fakeSudo(1)(name, args...) }),
				LookPath: fakeLookPath(nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			privilege.HasCaps = func() bool { return false }
			defer func() { privilege.HasCaps = privilege.RealHasCaps }()
			esc, err := privilege.NewWithRunner(context.Background(), tt.runner, zap.NewNop())
			if err != nil {
				t.Fatalf("privilege.NewWithRunner: %v", err)
			}
			err = esc.Ensure(context.Background())
			if err == nil {
				t.Fatalf("expected error")
			}
			if code := rootcmd.ExitCode(fmt.Errorf("privilege check failed: %w", err)); code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
		})
	}
}

// Helpers for external test package.

type cmdFunc func(context.Context, string, ...string) *exec.Cmd

func (f cmdFunc) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return f(ctx, name, args...)
}

func fakeLookPath(err error) func(string) (string, error) {
	return func(string) (string, error) { return "/usr/bin/sudo", err }
}

func fakeSudo(code int) func(string, ...string) *exec.Cmd {
	return func(string, ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", strconv.Itoa(code)}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
}

func requireSudo(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		t.Skip("sudo not available")
	}
}

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
}

// TestHelperProcess allows exec.Command stubbing.
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code, _ := strconv.Atoi(os.Args[len(os.Args)-1])
	os.Exit(code)
}
