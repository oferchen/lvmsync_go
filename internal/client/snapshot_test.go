package client_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"lvmsync_go/config"
	"lvmsync_go/internal/client"
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
