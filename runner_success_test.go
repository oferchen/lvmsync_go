package main

import (
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	"lvmsync_go/internal/exitcode"
)

func TestRunnerSuccess(t *testing.T) {
	logger := zap.NewNop()

	synced := false
	exitCalled := false
	exitCode := -1

	r := NewRunnerWithDeps(
		func() (*config.Config, []string, *zap.Logger, error) { return &config.Config{}, nil, logger, nil },
		func(_ *config.Config, _ []string, _ *zap.Logger) error { return nil },
		func(_ *zap.Logger) { synced = true },
		func(code int) { exitCalled = true; exitCode = code },
		func() *zap.Logger { return zap.NewNop() },
		"linux",
	)

	r.Run()

	if !synced {
		t.Fatalf("expected SyncLogger to be called")
	}

	if !exitCalled {
		t.Fatalf("expected Exit to be called")
	}
	if exitCode != exitcode.OK {
		t.Fatalf("expected exit code %d, got %d", exitcode.OK, exitCode)
	}
}
