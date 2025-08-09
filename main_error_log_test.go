package main

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type syncCheckCore struct {
	zapcore.Core
	synced *bool
}

func (c *syncCheckCore) Sync() error {
	*c.synced = true
	return c.Core.Sync()
}

func TestMainLogsStructuredError(t *testing.T) {
	// stub run to force an error
	oldRun := runFunc
	runFunc = func() error { return errors.New("boom") }
	defer func() { runFunc = oldRun }()

	// capture exit code
	oldExit := exitFunc
	var code int
	exitFunc = func(c int) { code = c }
	defer func() { exitFunc = oldExit }()

	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	logger := zap.New(&syncCheckCore{Core: core, synced: &synced})
	zap.ReplaceGlobals(logger)
	defer zap.ReplaceGlobals(zap.NewNop())

	main()

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	entries := logs.FilterMessage("run failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}

	if !synced {
		t.Fatalf("logger was not synced")
	}
}
