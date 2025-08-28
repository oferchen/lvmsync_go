package root

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestSetupSnapshotAndSignalsSuccess(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	snapCleanupCalled := false
	var snapshotPtr *string
	r := NewRunnerWithDeps(&Runner{
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			snapshotPtr = snap
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "snap-path", make(chan error), func() { snapCleanupCalled = true }, nil
		},
	})
	ctx, cleanup, snapPath, sigCh, monitorCh, err := r.setupSnapshotAndSignals(cfg, "/dev/vg/orig", zap.NewNop())
	if err != nil {
		t.Fatalf("setupSnapshotAndSignals error: %v", err)
	}
	if ctx == nil || cleanup == nil || sigCh == nil || monitorCh == nil {
		t.Fatalf("expected non-nil results")
	}
	if snapPath != "snap-path" {
		t.Fatalf("unexpected snapshot path: %s", snapPath)
	}
	if snapshotPtr == nil || *snapshotPtr != snapPath {
		t.Fatalf("snapshot pointer not updated")
	}
	cleanup()
	if !snapCleanupCalled {
		t.Fatalf("snapshot cleanup not called")
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("context not canceled after cleanup")
	}
}

func TestSetupSnapshotAndSignalsError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	r := NewRunnerWithDeps(&Runner{
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "", nil, nil, errors.New("snap error")
		},
	})
	ctx, cleanup, _, _, _, err := r.setupSnapshotAndSignals(cfg, "/dev/vg/orig", zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "prepare snapshot") {
		t.Fatalf("expected snapshot error, got: %v", err)
	}
	if ctx != nil || cleanup != nil {
		t.Fatalf("expected nil ctx and cleanup on error")
	}
}
