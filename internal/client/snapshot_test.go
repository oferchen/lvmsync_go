package client_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"lvmsync_go/internal/client"
	"lvmsync_go/internal/config"
	"lvmsync_go/lvm"

	"go.uber.org/zap"
)

func TestPrepareSkipSnapshot(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipSnapshotCreation = true
	cfg.Force = true
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"

	r := client.NewRunnerWithDeps(func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1, nil }, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	logger := zap.NewNop()
	snap, monitorCh, cleanup, err := r.PrepareSnapshot(context.Background(), cfg, "/dev/vg/orig", logger)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if snap != "/dev/vg/orig" {
		t.Fatalf("unexpected snapshot path: %s", snap)
	}
	if monitorCh != nil {
		t.Fatalf("expected nil monitor channel")
	}
	cleanup()
}

func TestPrepareSkipSnapshotRequiresForce(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipSnapshotCreation = true
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"

	r := client.NewRunnerWithDeps(func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1, nil }, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	logger := zap.NewNop()
	if _, _, _, err := r.PrepareSnapshot(context.Background(), cfg, "/dev/vg/orig", logger); err == nil {
		t.Fatalf("expected error when skipping snapshot without force")
	}
}

func TestPrepareSnapshotCreatesSnapshot(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"
	cfg.SnapshotSize = "25%"

	var parseArg string
	var created bool
	var removedPath string
	r := client.NewRunnerWithDeps(
		func(s, _ string, _ *lvm.FDCache, _ *zap.Logger) (uint64, error) {
			parseArg = s
			return 1024, nil
		},
		nil,
		nil,
		nil,
		nil,
		func(_ context.Context, _, _, size string, _ *zap.Logger) error {
			created = true
			if size != "1024" {
				t.Fatalf("unexpected size: %s", size)
			}
			return nil
		},
		func(name, vg string, _ *zap.Logger) string { return "/dev/" + vg + "/" + name },
		func(context.Context, string, float64, time.Duration, *zap.Logger) error { return nil },
		func(_ context.Context, path string, _ *zap.Logger) error {
			removedPath = path
			return nil
		},
		nil,
	)

	logger := zap.NewNop()
	snap, monitorCh, cleanup, err := r.PrepareSnapshot(context.Background(), cfg, "/dev/vg/orig", logger)
	if err != nil {
		t.Fatalf("PrepareSnapshot error: %v", err)
	}
	if parseArg != cfg.SnapshotSize {
		t.Fatalf("unexpected snapshot size arg %q", parseArg)
	}
	if !strings.HasPrefix(snap, "/dev/vg/snap-") {
		t.Fatalf("unexpected snapshot path: %s", snap)
	}
	if monitorCh == nil {
		t.Fatalf("expected monitor channel")
	}
	if !created {
		t.Fatalf("createSnapshot not invoked")
	}

	cleanup()
	if removedPath != snap {
		t.Fatalf("removeSnapshot called with %q, expected %q", removedPath, snap)
	}
}

func TestMonitorSnapshotUsageError(t *testing.T) {
	t.Parallel()
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"
	cfg.SnapshotMaxUsage = 0.5

	usageCh := make(chan float64, 1)
	done := make(chan struct{})
	var monCtx context.Context

	r := client.NewRunnerWithDeps(
		func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1024, nil },
		nil,
		nil,
		nil,
		nil,
		func(context.Context, string, string, string, *zap.Logger) error { return nil },
		func(name, vg string, _ *zap.Logger) string { return "/dev/" + vg + "/" + name },
		func(ctx context.Context, _ string, threshold float64, _ time.Duration, _ *zap.Logger) error {
			defer close(done)
			monCtx = ctx
			usage := <-usageCh
			if usage > threshold {
				return errors.New("usage exceeded")
			}
			return nil
		},
		func(context.Context, string, *zap.Logger) error { return nil },
		nil,
	)

	logger := zap.NewNop()
	_, monitorCh, cleanup, err := r.PrepareSnapshot(context.Background(), cfg, "/dev/vg/orig", logger)
	if err != nil {
		t.Fatalf("PrepareSnapshot error: %v", err)
	}

	usageCh <- cfg.SnapshotMaxUsage + 0.1

	select {
	case err, ok := <-monitorCh:
		if !ok {
			t.Fatalf("monitor channel closed without error")
		}
		if err == nil {
			t.Fatalf("expected error from monitor")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for monitor error")
	}

	if _, ok := <-monitorCh; ok {
		t.Fatalf("expected closed monitor channel")
	}

	cleanup()

	select {
	case <-monCtx.Done():
	case <-time.After(1 * time.Second):
		t.Fatalf("monitor context not canceled")
	}

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("monitor goroutine leaked")
	}
}

func TestSnapshotUsageAbortCleanup(t *testing.T) {
	t.Parallel()
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"
	cfg.SnapshotMaxUsage = 0.5

	usageCh := make(chan float64, 1)
	var removed bool

	r := client.NewRunnerWithDeps(
		func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1024, nil },
		nil,
		nil,
		nil,
		nil,
		func(context.Context, string, string, string, *zap.Logger) error { return nil },
		func(name, vg string, _ *zap.Logger) string { return "/dev/" + vg + "/" + name },
		func(ctx context.Context, _ string, threshold float64, _ time.Duration, _ *zap.Logger) error {
			usage := <-usageCh
			if usage > threshold {
				return errors.New("snapshot exhausted")
			}
			return nil
		},
		func(context.Context, string, *zap.Logger) error {
			removed = true
			return nil
		},
		func() string { return "snap" },
	)

	logger := zap.NewNop()
	snap, monitorCh, cleanup, err := r.PrepareSnapshot(context.Background(), cfg, "/dev/vg/orig", logger)
	if err != nil {
		t.Fatalf("PrepareSnapshot error: %v", err)
	}

	abortCh := make(chan struct{})
	var walUpdated, manifestUpdated bool
	runClient := func(ctx context.Context, snapshotPath, destPath string) error {
		select {
		case <-abortCh:
			return nil
		case <-time.After(50 * time.Millisecond):
			walUpdated = true
			manifestUpdated = true
			return nil
		}
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ExecuteClient(context.Background(), runClient, snap, "dest", nil, monitorCh, logger)
	}()

	usageCh <- cfg.SnapshotMaxUsage + 0.1
	close(abortCh)

	err = <-errCh
	if err == nil || !strings.Contains(err.Error(), "snapshot exhausted") {
		t.Fatalf("expected snapshot exhausted error, got %v", err)
	}

	cleanup()
	if !removed {
		t.Fatalf("snapshot not cleaned up")
	}
	if walUpdated || manifestUpdated {
		t.Fatalf("unexpected WAL or manifest updates after abort")
	}
}

func TestCreateSnapshotCleanupNoPanic(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"

	ready := make(chan struct{}, 1)
	r := client.NewRunnerWithDeps(
		func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1024, nil },
		nil,
		nil,
		nil,
		nil,
		func(context.Context, string, string, string, *zap.Logger) error { return nil },
		func(name, vg string, _ *zap.Logger) string { return "/dev/" + vg + "/" + name },
		func(ctx context.Context, _ string, _ float64, _ time.Duration, _ *zap.Logger) error {
			ready <- struct{}{}
			<-ctx.Done()
			return errors.New("monitor error")
		},
		func(context.Context, string, *zap.Logger) error { return nil },
		nil,
	)

	logger := zap.NewNop()
	_, monitorCh, cleanup, err := r.PrepareSnapshot(context.Background(), cfg, "/dev/vg/orig", logger)
	if err != nil {
		t.Fatalf("PrepareSnapshot error: %v", err)
	}

	<-ready
	cleanup()

	select {
	case err, ok := <-monitorCh:
		if !ok {
			t.Fatalf("expected monitor error")
		}
		if err == nil {
			t.Fatalf("expected error from monitor, got nil")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for monitor error")
	}

	if _, ok := <-monitorCh; ok {
		t.Fatalf("expected closed monitor channel")
	}
}

func TestPrepareSnapshotCreateSnapshotError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"

	r := client.NewRunnerWithDeps(
		func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1024, nil },
		nil,
		nil,
		nil,
		nil,
		func(context.Context, string, string, string, *zap.Logger) error { return errors.New("create error") },
		nil,
		nil,
		nil,
		nil,
	)

	logger := zap.NewNop()
	if _, _, _, err := r.PrepareSnapshot(context.Background(), cfg, "/dev/vg/orig", logger); err == nil {
		t.Fatalf("expected create error")
	}
}
