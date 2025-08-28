package signals

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestHandleInterruptNoCleanup(t *testing.T) {
	cfg := &config.Config{SkipSnapshotCreation: true}
	sigCh := make(chan os.Signal, 1)
	errCh := make(chan error, 1)
	snap := "lvmsnap"
	var called atomic.Bool
	runner := NewRunnerWithDeps(func(context.Context, string, *zap.Logger) error {
		called.Store(true)
		return nil
	})
	go runner.Handle(context.Background(), cfg, zap.NewNop(), sigCh, &snap, errCh)
	sigCh <- os.Interrupt
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "received signal") {
		t.Fatalf("expected signal error, got %v", err)
	}
	if called.Load() {
		t.Fatal("RemoveSnapshot should not be called when SkipSnapshotCreation is true")
	}
}

func TestHandleTerminationCleanup(t *testing.T) {
	cfg := &config.Config{SkipSnapshotCreation: false}
	sigCh := make(chan os.Signal, 1)
	errCh := make(chan error, 1)
	snap := "snapA"
	var called atomic.Bool
	runner := NewRunnerWithDeps(func(_ context.Context, path string, _ *zap.Logger) error {
		if path != snap {
			t.Fatalf("unexpected snapshot path %q", path)
		}
		called.Store(true)
		return nil
	})
	go runner.Handle(context.Background(), cfg, zap.NewNop(), sigCh, &snap, errCh)
	sigCh <- syscall.SIGTERM
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "terminated") {
		t.Fatalf("expected terminated signal error, got %v", err)
	}
	if !called.Load() {
		t.Fatal("RemoveSnapshot was not called")
	}
}

func TestHandleCleanupFailure(t *testing.T) {
	cfg := &config.Config{SkipSnapshotCreation: false}
	sigCh := make(chan os.Signal, 1)
	errCh := make(chan error, 1)
	snap := "snapB"
	var count atomic.Int32
	removeErr := errors.New("boom")
	runner := NewRunnerWithDeps(func(context.Context, string, *zap.Logger) error {
		count.Add(1)
		return removeErr
	})
	go runner.Handle(context.Background(), cfg, zap.NewNop(), sigCh, &snap, errCh)
	sigCh <- os.Interrupt
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "interrupt") {
		t.Fatalf("expected interrupt signal error, got %v", err)
	}
	if count.Load() != 1 {
		t.Fatalf("RemoveSnapshot called %d times, want 1", count.Load())
	}
}
