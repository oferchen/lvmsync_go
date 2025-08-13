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
	err    error
}

func (c *syncCheckCore) Sync() error {
	*c.synced = true
	if c.err != nil {
		return c.err
	}
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

	syncErr := errors.New("sync fail")
	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	logger := zap.New(&syncCheckCore{Core: core, synced: &synced, err: syncErr})

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

	syncEntries := logs.FilterMessage("Logger sync error").All()
	if len(syncEntries) != 1 {
		t.Fatalf("expected logger sync error entry, got %d", len(syncEntries))
	}
	errStr, ok := syncEntries[0].ContextMap()["error"].(string)
	if !ok || errStr != syncErr.Error() {
		t.Fatalf("expected error %q in log, got %v", syncErr.Error(), syncEntries[0].ContextMap()["error"])
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

	syncErr := errors.New("sync fail")
	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	tmpLogger := zap.New(&syncCheckCore{Core: core, synced: &synced, err: syncErr})

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

	syncEntries := logs.FilterMessage("Logger sync error").All()
	if len(syncEntries) != 1 {
		t.Fatalf("expected logger sync error entry, got %d", len(syncEntries))
	}
	errStr, ok := syncEntries[0].ContextMap()["error"].(string)
	if !ok || errStr != syncErr.Error() {
		t.Fatalf("expected error %q in log, got %v", syncErr.Error(), syncEntries[0].ContextMap()["error"])
	}

	if !synced {
		t.Fatalf("logger was not synced")
	}
}

func TestMainErrorsOnNonLinux(t *testing.T) {
	oldGOOS := runtimeGOOS
	runtimeGOOS = "darwin"
	defer func() { runtimeGOOS = oldGOOS }()

	oldExit := exitFunc
	var code int
	exitFunc = func(c int) { code = c }
	defer func() { exitFunc = oldExit }()

	syncErr := errors.New("sync fail")
	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	tmpLogger := zap.New(&syncCheckCore{Core: core, synced: &synced, err: syncErr})

	oldExample := exampleLoggerFunc
	exampleLoggerFunc = func() *zap.Logger { return tmpLogger }
	defer func() { exampleLoggerFunc = oldExample }()

	main()

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	entries := logs.FilterMessage("unsupported platform").All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}

	syncEntries := logs.FilterMessage("Logger sync error").All()
	if len(syncEntries) != 1 {
		t.Fatalf("expected logger sync error entry, got %d", len(syncEntries))
	}
	errStr, ok := syncEntries[0].ContextMap()["error"].(string)
	if !ok || errStr != syncErr.Error() {
		t.Fatalf("expected error %q in log, got %v", syncErr.Error(), syncEntries[0].ContextMap()["error"])
	}

	if !synced {
		t.Fatalf("logger was not synced")
	}
}
