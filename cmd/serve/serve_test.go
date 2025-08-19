package main

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestServeCommandDeprecated(t *testing.T) {
	syncCalled := false
	exitCode := 0
	core, logs := observer.New(zapcore.ErrorLevel)
	r := newRunnerWithDeps(
		nil,
		func(*zap.Logger) { syncCalled = true },
		func(c int) { exitCode = c },
		func() *zap.Logger { return zap.New(core) },
	)
	r.Run(nil)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	entries := logs.FilterMessage("command_deprecated").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	replacement, ok := entries[0].ContextMap()["replacement"].(string)
	if !ok || replacement != "lvmsyncd" {
		t.Fatalf("expected replacement lvmsyncd, got %v", entries[0].ContextMap()["replacement"])
	}
	if !syncCalled {
		t.Fatalf("expected SyncLogger to be called")
	}
}
