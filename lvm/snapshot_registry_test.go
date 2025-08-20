package lvm

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

func TestRegisterSnapshotNilLoggerPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
	}()
	RegisterSnapshot("/dev/test", nil)
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

func TestCleanupSnapshotNilLoggerPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
	}()
	CleanupSnapshot(context.Background(), "/dev/test", nil)
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
