package lvm

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCleanupSnapshotSuccess(t *testing.T) {
	original := removeSnap
	defer func() { removeSnap = original }()

	var called bool
	removeSnap = func(ctx context.Context, path string, logger *zap.Logger) error {
		called = true
		if path != "/dev/vg/test" {
			t.Fatalf("removeSnap called with %s", path)
		}
		return nil
	}

	registryMu.Lock()
	registry = map[string]*zap.Logger{"/dev/vg/test": zap.NewNop()}
	registryMu.Unlock()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	CleanupSnapshot(context.Background(), "/dev/vg/test", logger)

	if !called {
		t.Fatal("removeSnap was not called")
	}

	registryMu.Lock()
	if _, ok := registry["/dev/vg/test"]; ok {
		registryMu.Unlock()
		t.Fatal("snapshot still registered after cleanup")
	}
	registryMu.Unlock()

	entries := logs.FilterMessage("snapshot removed").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if v, ok := entries[0].ContextMap()["snapshot"].(string); !ok || v != "/dev/vg/test" {
		t.Fatalf("unexpected snapshot field: %v", entries[0].ContextMap()["snapshot"])
	}
}

func TestCleanupSnapshotError(t *testing.T) {
	original := removeSnap
	defer func() { removeSnap = original }()

	removeSnap = func(ctx context.Context, path string, logger *zap.Logger) error {
		return errors.New("boom")
	}

	registryMu.Lock()
	registry = map[string]*zap.Logger{"/dev/vg/test": zap.NewNop()}
	registryMu.Unlock()

	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	CleanupSnapshot(context.Background(), "/dev/vg/test", logger)

	registryMu.Lock()
	if _, ok := registry["/dev/vg/test"]; ok {
		registryMu.Unlock()
		t.Fatal("snapshot still registered after cleanup")
	}
	registryMu.Unlock()

	entries := logs.FilterMessage("failed to remove snapshot").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if v, ok := entries[0].ContextMap()["snapshot"].(string); !ok || v != "/dev/vg/test" {
		t.Fatalf("unexpected snapshot field: %v", entries[0].ContextMap()["snapshot"])
	}
}

func TestCleanupRegisteredMultiple(t *testing.T) {
	original := removeSnap
	defer func() { removeSnap = original }()

	var calls []string
	removeSnap = func(ctx context.Context, path string, logger *zap.Logger) error {
		calls = append(calls, path)
		return nil
	}

	core1, logs1 := observer.New(zap.InfoLevel)
	logger1 := zap.New(core1)
	core2, logs2 := observer.New(zap.InfoLevel)
	logger2 := zap.New(core2)

	registryMu.Lock()
	registry = map[string]*zap.Logger{
		"/snap/1": logger1,
		"/snap/2": logger2,
	}
	registryMu.Unlock()

	CleanupRegistered(context.Background())

	if len(calls) != 2 {
		t.Fatalf("expected 2 removeSnap calls, got %d", len(calls))
	}
	got := map[string]bool{}
	for _, c := range calls {
		got[c] = true
	}
	if !got["/snap/1"] || !got["/snap/2"] {
		t.Fatalf("expected calls for both snapshots, got %v", calls)
	}

	if len(logs1.FilterMessage("snapshot removed").All()) != 1 {
		t.Fatalf("logger1 expected 1 log entry")
	}
	if len(logs2.FilterMessage("snapshot removed").All()) != 1 {
		t.Fatalf("logger2 expected 1 log entry")
	}

	registryMu.Lock()
	if len(registry) != 0 {
		registryMu.Unlock()
		t.Fatalf("registry not cleared: %v", registry)
	}
	registryMu.Unlock()
}

func TestRegisterSnapshotNilLoggerDefaults(t *testing.T) {
	registryMu.Lock()
	registry = make(map[string]*zap.Logger)
	registryMu.Unlock()

	RegisterSnapshot("/dev/test", nil)

	registryMu.Lock()
	l, ok := registry["/dev/test"]
	registry = make(map[string]*zap.Logger)
	registryMu.Unlock()

	if !ok {
		t.Fatalf("snapshot not registered")
	}
	if l == nil {
		t.Fatalf("logger not defaulted")
	}
}

func TestRegisterSnapshotAddsSnapshot(t *testing.T) {
	registryMu.Lock()
	registry = make(map[string]*zap.Logger)
	registryMu.Unlock()

	RegisterSnapshot("/dev/test", zap.NewNop())

	registryMu.Lock()
	_, ok := registry["/dev/test"]
	registry = make(map[string]*zap.Logger)
	registryMu.Unlock()

	if !ok {
		t.Fatalf("snapshot not registered")
	}
}

func TestCleanupSnapshotNilLoggerDefaults(t *testing.T) {
	var got *zap.Logger
	old := removeSnap
	removeSnap = func(ctx context.Context, path string, logger *zap.Logger) error {
		got = logger
		return nil
	}
	defer func() { removeSnap = old }()

	CleanupSnapshot(context.Background(), "/dev/test", nil)

	if got == nil {
		t.Fatalf("logger not defaulted")
	}
}

func TestCleanupSnapshotCallsRemove(t *testing.T) {
	var called bool
	old := removeSnap
	removeSnap = func(ctx context.Context, path string, logger *zap.Logger) error {
		called = true
		return nil
	}
	defer func() { removeSnap = old }()

	CleanupSnapshot(context.Background(), "/dev/test", zap.NewNop())

	if !called {
		t.Fatalf("removeSnap not called")
	}
}
