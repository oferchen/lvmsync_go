package privilege

import (
	"context"
	"os/exec"
	"testing"

	"go.uber.org/zap"
)

func TestEnsureRealSudo(t *testing.T) {
	requireSudo(t)
	HasCaps = func() bool { return false }
	esc := New(context.Background(), zap.NewNop())
	if err := esc.Ensure(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSudoCommandRealSuccess(t *testing.T) {
	requireSudo(t)
	HasCaps = func() bool { return false }
	esc := New(context.Background(), zap.NewNop())
	cmd := esc.Command(context.Background(), "true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSudoCommandRealFailure(t *testing.T) {
	requireSudo(t)
	HasCaps = func() bool { return false }
	esc := New(context.Background(), zap.NewNop())
	cmd := esc.Command(context.Background(), "false")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected error")
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		t.Fatalf("unexpected error: %v", err)
	}
}
