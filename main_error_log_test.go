package main

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/config"
	"lvmsync_go/internal/exitcode"
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
	syncErr := errors.New("sync fail")
	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	logger := zap.New(&syncCheckCore{Core: core, synced: &synced, err: syncErr})

	var code int
	runner := NewRunnerWithDeps(
		func() (*config.Config, []string, *zap.Logger, error) { return &config.Config{}, nil, logger, nil },
		func(_ *config.Config, _ []string, _ *zap.Logger) error { return errors.New("boom") },
		rootcmd.SyncLogger,
		func(c int) { code = c },
		func() *zap.Logger { return zap.NewNop() },
		"linux",
	)
	runner.Run()

	if code != exitcode.ErrRuntime {
		t.Fatalf("expected exit code %d, got %d", exitcode.ErrRuntime, code)
	}

	entries := logs.FilterMessage("run failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}

	syncEntries := logs.FilterMessage("logger_sync_error").All()
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
	syncErr := errors.New("sync fail")
	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	tmpLogger := zap.New(&syncCheckCore{Core: core, synced: &synced, err: syncErr})

	var code int
	runner := NewRunnerWithDeps(
		func() (*config.Config, []string, *zap.Logger, error) {
			return nil, nil, nil, errors.New("config invalid")
		},
		rootcmd.Run,
		rootcmd.SyncLogger,
		func(c int) { code = c },
		func() *zap.Logger { return tmpLogger },
		"linux",
	)
	runner.Run()

	if code != exitcode.ErrConfig {
		t.Fatalf("expected exit code %d, got %d", exitcode.ErrConfig, code)
	}

	entries := logs.FilterMessage("configuration failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}

	syncEntries := logs.FilterMessage("logger_sync_error").All()
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
	syncErr := errors.New("sync fail")
	core, logs := observer.New(zap.ErrorLevel)
	synced := false
	tmpLogger := zap.New(&syncCheckCore{Core: core, synced: &synced, err: syncErr})

	var code int
	runner := NewRunnerWithDeps(
		rootcmd.Configure,
		rootcmd.Run,
		rootcmd.SyncLogger,
		func(c int) { code = c },
		func() *zap.Logger { return tmpLogger },
		"darwin",
	)
	runner.Run()

	if code != exitcode.ErrPlatform {
		t.Fatalf("expected exit code %d, got %d", exitcode.ErrPlatform, code)
	}

	entries := logs.FilterMessage("unsupported platform").All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}

	syncEntries := logs.FilterMessage("logger_sync_error").All()
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

func TestMainCapabilityErrorExitCode(t *testing.T) {
	var code int
	runner := NewRunnerWithDeps(
		func() (*config.Config, []string, *zap.Logger, error) {
			return nil, nil, nil, errors.New("privilege check failed: caps")
		},
		rootcmd.Run,
		rootcmd.SyncLogger,
		func(c int) { code = c },
		func() *zap.Logger { return zap.NewNop() },
		"linux",
	)
	runner.Run()
	if code != exitcode.ErrCapability {
		t.Fatalf("expected exit code %d, got %d", exitcode.ErrCapability, code)
	}
}

func TestMainDeviceErrorExitCode(t *testing.T) {
	var code int
	logger := zap.NewNop()
	runner := NewRunnerWithDeps(
		func() (*config.Config, []string, *zap.Logger, error) { return &config.Config{}, nil, logger, nil },
		func(_ *config.Config, _ []string, _ *zap.Logger) error { return errors.New("device lost") },
		rootcmd.SyncLogger,
		func(c int) { code = c },
		func() *zap.Logger { return zap.NewNop() },
		"linux",
	)
	runner.Run()
	if code != exitcode.ErrDevice {
		t.Fatalf("expected exit code %d, got %d", exitcode.ErrDevice, code)
	}
}

func TestMainVerifyExitCode(t *testing.T) {
	var code int
	logger := zap.NewNop()
	runner := NewRunnerWithDeps(
		func() (*config.Config, []string, *zap.Logger, error) { return &config.Config{}, nil, logger, nil },
		func(_ *config.Config, _ []string, _ *zap.Logger) error { return errors.New("digest mismatch") },
		rootcmd.SyncLogger,
		func(c int) { code = c },
		func() *zap.Logger { return zap.NewNop() },
		"linux",
	)
	runner.Run()
	if code != exitcode.ErrVerify {
		t.Fatalf("expected exit code %d, got %d", exitcode.ErrVerify, code)
	}
}

func TestMainPartialExitCode(t *testing.T) {
	var code int
	logger := zap.NewNop()
	runner := NewRunnerWithDeps(
		func() (*config.Config, []string, *zap.Logger, error) { return &config.Config{}, nil, logger, nil },
		func(_ *config.Config, _ []string, _ *zap.Logger) error {
			return errors.New("received signal: interrupt")
		},
		rootcmd.SyncLogger,
		func(c int) { code = c },
		func() *zap.Logger { return zap.NewNop() },
		"linux",
	)
	runner.Run()
	if code != exitcode.ErrPartial {
		t.Fatalf("expected exit code %d, got %d", exitcode.ErrPartial, code)
	}
}
