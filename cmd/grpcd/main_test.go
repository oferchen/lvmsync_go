package main

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/exitcode"
)

func TestRunLoggerError(t *testing.T) {
	code := run(func() (*zap.Logger, error) { return nil, errors.New("logger fail") }, NewRunner())
	if code != exitcode.ErrConfig {
		t.Fatalf("expected %d, got %d", exitcode.ErrConfig, code)
	}
}

func TestRunExecuteError(t *testing.T) {
	r := NewRunnerWithDeps(func(_ context.Context, _ Options, _ *zap.Logger) error {
		return errors.New("boom")
	})
	code := run(func() (*zap.Logger, error) { return zap.NewNop(), nil }, r)
	if code != exitcode.ErrRuntime {
		t.Fatalf("expected %d, got %d", exitcode.ErrRuntime, code)
	}
}
