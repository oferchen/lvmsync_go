package main

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
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
	runFunc = func(_ *config.Config, _ *zap.Logger) error { return errors.New("boom") }
	defer func() { runFunc = oldRun }()

	// capture exit code
	oldExit := exitFunc
	var code int
	exitFunc = func(c int) { code = c }
	defer func() { exitFunc = oldExit }()

	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	logger := zap.New(&syncCheckCore{Core: core, synced: &synced})

	oldConfigure := configureFunc
	configureFunc = func() (*config.Config, *zap.Logger, error) { return &config.Config{}, logger, nil }
	defer func() { configureFunc = oldConfigure }()

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

func TestMainLogsConfigError(t *testing.T) {
	// stub configure to return an error
	oldConfigure := configureFunc
	configureFunc = func() (*config.Config, *zap.Logger, error) { return nil, nil, errors.New("cfg fail") }
	defer func() { configureFunc = oldConfigure }()

	// capture exit code
	oldExit := exitFunc
	var code int
	exitFunc = func(c int) { code = c }
	defer func() { exitFunc = oldExit }()

	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	tmpLogger := zap.New(&syncCheckCore{Core: core, synced: &synced})

	oldExample := exampleLoggerFunc
	exampleLoggerFunc = func() *zap.Logger { return tmpLogger }
	defer func() { exampleLoggerFunc = oldExample }()

	main()

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	entries := logs.FilterMessage("configuration failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}

	if !synced {
		t.Fatalf("logger was not synced")
	}
}
