package root

import (
	"context"
	"errors"
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	"lvmsync_go/transport"
)

func TestPrepareClientCreatesSnapshotAndCleanup(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}

	snapCleanupCalled := false
	var snapshotPtr *string
	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			snapshotPtr = snap
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "snap-path", make(chan error), func() { snapCleanupCalled = true }, nil
		},
	})

	logger := zap.NewNop()
	ctx, cleanup, snapPath, destPath, sigCh, monitorCh, err := r.prepareClient(cfg, []string{"/dev/vg/orig", "/dest"}, logger)
	if err != nil {
		t.Fatalf("prepareClient error: %v", err)
	}
	if ctx == nil || cleanup == nil || sigCh == nil || monitorCh == nil {
		t.Fatalf("expected non-nil results")
	}
	if snapPath != "snap-path" {
		t.Fatalf("unexpected snapshot path: %s", snapPath)
	}
	if destPath != "/dest" {
		t.Fatalf("unexpected dest path: %s", destPath)
	}
	if snapshotPtr == nil || *snapshotPtr != snapPath {
		t.Fatalf("snapshot path pointer not updated")
	}

	cleanup()
	if !snapCleanupCalled {
		t.Fatalf("snapshot cleanup not invoked")
	}
}

func TestPrepareClientSnapshotError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}

	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "", nil, nil, errors.New("snap error")
		},
	})

	logger := zap.NewNop()
	if _, cleanup, _, _, _, _, err := r.prepareClient(cfg, []string{"/dev/vg/orig", "/dest"}, logger); err == nil {
		t.Fatalf("expected snapshot error")
	} else if cleanup != nil {
		t.Fatalf("expected nil cleanup on error")
	}
}
