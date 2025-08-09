package client_test

import (
	"testing"

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
