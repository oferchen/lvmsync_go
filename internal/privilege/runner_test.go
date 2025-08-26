package privilege

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestNewWithSanitize(t *testing.T) {
	esc := NewWithSanitize(context.Background(), true).(*sudoEscalator)
	if !esc.sanitizeEnv {
		t.Fatalf("expected sanitizeEnv to be true")
	}
	if esc.runner == nil || esc.runner.Cmd == nil || esc.runner.LookPath == nil || esc.runner.Logger == nil {
		t.Fatalf("runner not fully populated: %+v", esc.runner)
	}
}

func TestNewWithRunnerAndSanitize(t *testing.T) {
	r := &Runner{
		Cmd:      commanderFunc(noopCmd),
		LookPath: func(string) (string, error) { return "/usr/bin/true", nil },
		Logger:   zap.NewNop(),
	}
	esc := NewWithRunnerAndSanitize(context.Background(), r, true).(*sudoEscalator)
	if !esc.sanitizeEnv {
		t.Fatalf("expected sanitizeEnv to be true")
	}
	if esc.runner == nil {
		t.Fatalf("runner is nil")
	}
	if esc.runner.Cmd == nil || esc.runner.LookPath == nil {
		t.Fatalf("runner missing dependencies: %+v", esc.runner)
	}
	if esc.runner.Logger != r.Logger {
		t.Fatalf("logger not propagated")
	}
}

func TestNewNilLogger(t *testing.T) {
	if _, err := New(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "logger is nil") {
		t.Fatalf("expected nil logger error, got %v", err)
	}
}
