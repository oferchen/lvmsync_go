package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunGeneratesDoc(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	path := filepath.Join(tmp, "docs", "config_env.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected doc at %s: %v", path, err)
	}
}

func TestRunMissingGoMod(t *testing.T) {
	tmp := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := run(); err == nil {
		t.Fatal("expected error")
	}
}

func TestMainLogsAndSyncs(t *testing.T) {
	syncCalled := false
	exitCode := 0
	core, logs := observer.New(zapcore.ErrorLevel)
	r := newRunnerWithDeps(
		func() error { return errors.New("fail") },
		func(*zap.Logger) { syncCalled = true },
		func(c int) { exitCode = c },
		func() *zap.Logger { return zap.New(core) },
	)
	r.Run()
	if exitCode != 1 {
		t.Fatalf("exit code: got %d want 1", exitCode)
	}
	entries := logs.FilterMessage("run_failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if !syncCalled {
		t.Fatalf("expected SyncLogger to be called")
	}
}
