package lvm

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCleanupSnapshotSuccess(t *testing.T) {
	var called bool
	sr := NewSnapshotRegistry(func(ctx context.Context, path string, logger *zap.Logger) error {
		called = true
		if path != "/dev/vg/test" {
			t.Fatalf("remove called with %s", path)
		}
		return nil
	})
	sr.RegisterSnapshot("/dev/vg/test", zap.NewNop())

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	sr.CleanupSnapshot(context.Background(), "/dev/vg/test", logger)

	if !called {
		t.Fatal("remove not called")
	}
	sr.mu.Lock()
	if _, ok := sr.registry["/dev/vg/test"]; ok {
		sr.mu.Unlock()
		t.Fatal("snapshot still registered")
	}
	sr.mu.Unlock()
	entries := logs.FilterMessage("snapshot removed").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if v, ok := entries[0].ContextMap()["snapshot"].(string); !ok || v != "/dev/vg/test" {
		t.Fatalf("unexpected snapshot field: %v", entries[0].ContextMap()["snapshot"])
	}
}

func TestCleanupSnapshotError(t *testing.T) {
	sr := NewSnapshotRegistry(func(ctx context.Context, path string, logger *zap.Logger) error {
		return errors.New("boom")
	})
	sr.RegisterSnapshot("/dev/vg/test", zap.NewNop())

	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	sr.CleanupSnapshot(context.Background(), "/dev/vg/test", logger)

	sr.mu.Lock()
	if _, ok := sr.registry["/dev/vg/test"]; ok {
		sr.mu.Unlock()
		t.Fatal("snapshot still registered")
	}
	sr.mu.Unlock()
	entries := logs.FilterMessage("failed to remove snapshot").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if v, ok := entries[0].ContextMap()["snapshot"].(string); !ok || v != "/dev/vg/test" {
		t.Fatalf("unexpected snapshot field: %v", entries[0].ContextMap()["snapshot"])
	}
}

func TestCleanupRegisteredMultiple(t *testing.T) {
	var calls []string
	sr := NewSnapshotRegistry(func(ctx context.Context, path string, logger *zap.Logger) error {
		calls = append(calls, path)
		return nil
	})
	core1, logs1 := observer.New(zap.InfoLevel)
	logger1 := zap.New(core1)
	core2, logs2 := observer.New(zap.InfoLevel)
	logger2 := zap.New(core2)
	sr.RegisterSnapshot("/snap/1", logger1)
	sr.RegisterSnapshot("/snap/2", logger2)

	sr.CleanupRegistered(context.Background())

	if len(calls) != 2 {
		t.Fatalf("expected 2 remove calls, got %d", len(calls))
	}
	got := map[string]bool{}
	for _, c := range calls {
		got[c] = true
	}
	if !got["/snap/1"] || !got["/snap/2"] {
		t.Fatalf("unexpected calls %v", calls)
	}
	if len(logs1.FilterMessage("snapshot removed").All()) != 1 {
		t.Fatalf("logger1 expected 1 log entry")
	}
	if len(logs2.FilterMessage("snapshot removed").All()) != 1 {
		t.Fatalf("logger2 expected 1 log entry")
	}
	sr.mu.Lock()
	if len(sr.registry) != 0 {
		sr.mu.Unlock()
		t.Fatalf("registry not cleared: %v", sr.registry)
	}
	sr.mu.Unlock()
}

func TestRegisterSnapshotAddsSnapshot(t *testing.T) {
	sr := NewSnapshotRegistry(nil)
	sr.RegisterSnapshot("/dev/test", zap.NewNop())
	sr.mu.Lock()
	_, ok := sr.registry["/dev/test"]
	sr.registry = make(map[string]*zap.Logger)
	sr.mu.Unlock()
	if !ok {
		t.Fatalf("snapshot not registered")
	}
}
