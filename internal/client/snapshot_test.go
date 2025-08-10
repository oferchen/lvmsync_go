package client_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"lvmsync_go/config"
	"lvmsync_go/internal/client"

	"go.uber.org/zap"
)

func TestPrepareSkipSnapshot(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipSnapshotCreation = true
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"

	restore := client.SetParseSnapshotSizeForTest(func(string, string) (uint64, error) { return 1, nil })
	defer restore()

	logger := zap.NewNop()
	snap, monitorCh, cleanup, err := client.PrepareSnapshot(cfg, "/dev/vg/orig", logger)
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

func TestPrepareSnapshotCreatesSnapshot(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"

	restoreParse := client.SetParseSnapshotSizeForTest(func(string, string) (uint64, error) { return 1024, nil })
	defer restoreParse()

	var created bool
	restoreCreate := client.SetCreateSnapshotForTest(func(ctx context.Context, orig, name, size string) error {
		created = true
		if size != "1024" {
			t.Fatalf("unexpected size: %s", size)
		}
		return nil
	})
	defer restoreCreate()

	restorePath := client.SetGetSnapshotDevicePathForTest(func(name, vg string) string {
		return "/dev/" + vg + "/" + name
	})
	defer restorePath()

	restoreMonitor := client.SetMonitorSnapshotForTest(func(ctx context.Context, path string, threshold float64, interval time.Duration) error {
		return nil
	})
	defer restoreMonitor()

	var removedPath string
	restoreRemove := client.SetRemoveSnapshotForTest(func(ctx context.Context, path string) error {
		removedPath = path
		return nil
	})
	defer restoreRemove()

	logger := zap.NewNop()
	snap, monitorCh, cleanup, err := client.PrepareSnapshot(cfg, "/dev/vg/orig", logger)
	if err != nil {
		t.Fatalf("PrepareSnapshot error: %v", err)
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

func TestCreateSnapshotCleanupNoPanic(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"

	restoreParse := client.SetParseSnapshotSizeForTest(func(string, string) (uint64, error) { return 1024, nil })
	defer restoreParse()

	restoreCreate := client.SetCreateSnapshotForTest(func(context.Context, string, string, string) error { return nil })
	defer restoreCreate()

	restorePath := client.SetGetSnapshotDevicePathForTest(func(name, vg string) string { return "/dev/" + vg + "/" + name })
	defer restorePath()

	ready := make(chan struct{}, 1)
	restoreMonitor := client.SetMonitorSnapshotForTest(func(ctx context.Context, path string, threshold float64, interval time.Duration) error {
		ready <- struct{}{}
		<-ctx.Done()
		return errors.New("monitor error")
	})
	defer restoreMonitor()

	restoreRemove := client.SetRemoveSnapshotForTest(func(context.Context, string) error { return nil })
	defer restoreRemove()

	logger := zap.NewNop()
	_, monitorCh, cleanup, err := client.PrepareSnapshot(cfg, "/dev/vg/orig", logger)
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
