package snapshot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/lvm"
)

func TestEnsureVolumeGroups(t *testing.T) {
	logger := zap.NewNop()

	t.Run("sets missing groups", func(t *testing.T) {
		origGetVG := getVolumeGroupName
		origGetSize := getVolumeSize
		origSelect := selectVolumeGroupForSize
		defer func() {
			getVolumeGroupName = origGetVG
			getVolumeSize = origGetSize
			selectVolumeGroupForSize = origSelect
		}()

		getVolumeGroupName = func(string) (string, error) { return "sourcevg", nil }
		getVolumeSize = func(string) (uint64, error) { return 10, nil }
		selectVolumeGroupForSize = func(context.Context, []string, uint64) (lvm.VolumeGroup, error) {
			return lvm.VolumeGroup{Name: "targetvg"}, nil
		}

		cfg := &config.Config{TargetVGCandidates: []string{"t1", "t2"}}
		if err := ensureVolumeGroups(cfg, "/dev/sourcevg/lv", logger); err != nil {
			t.Fatalf("ensureVolumeGroups error: %v", err)
		}
		if cfg.VolumeGroup != "sourcevg" {
			t.Fatalf("expected volume group 'sourcevg', got %q", cfg.VolumeGroup)
		}
		if cfg.TargetVolumeGroup != "targetvg" {
			t.Fatalf("expected target volume group 'targetvg', got %q", cfg.TargetVolumeGroup)
		}
	})

	t.Run("preserves existing groups", func(t *testing.T) {
		origGetVG := getVolumeGroupName
		origSelect := selectVolumeGroupForSize
		defer func() {
			getVolumeGroupName = origGetVG
			selectVolumeGroupForSize = origSelect
		}()

		calledGet := false
		calledSelect := false
		getVolumeGroupName = func(string) (string, error) { calledGet = true; return "vg", nil }
		selectVolumeGroupForSize = func(context.Context, []string, uint64) (lvm.VolumeGroup, error) {
			calledSelect = true
			return lvm.VolumeGroup{Name: "vg"}, nil
		}

		cfg := &config.Config{VolumeGroup: "existing", TargetVolumeGroup: "target"}
		if err := ensureVolumeGroups(cfg, "/dev/existing/lv", logger); err != nil {
			t.Fatalf("ensureVolumeGroups error: %v", err)
		}
		if calledGet || calledSelect {
			t.Fatalf("unexpected call to helpers when groups already set")
		}
	})
}

func TestCheckDiskSpaceForSnapshot(t *testing.T) {
	logger := zap.NewNop()
	origCheck := checkDiskSpace
	defer func() { checkDiskSpace = origCheck }()

	cfg := &config.Config{SkipDiskCheck: false}

	checkDiskSpace = func(string) (uint64, error) { return 1024, nil }
	if err := checkDiskSpaceForSnapshot(cfg, 512, logger); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	checkDiskSpace = func(string) (uint64, error) { return 100, nil }
	if err := checkDiskSpaceForSnapshot(cfg, 200, logger); err == nil {
		t.Fatalf("expected insufficient space error")
	}

	cfg.SkipDiskCheck = true
	called := false
	checkDiskSpace = func(string) (uint64, error) { called = true; return 0, errors.New("boom") }
	if err := checkDiskSpaceForSnapshot(cfg, 1<<20, logger); err != nil {
		t.Fatalf("expected nil when skip disk check, got %v", err)
	}
	if called {
		t.Fatalf("checkDiskSpace should not be called when skip enabled")
	}
}

func TestCreateSnapshotIfNeeded(t *testing.T) {
	logger := zap.NewNop()

	t.Run("skip creation", func(t *testing.T) {
		cfg := &config.Config{SkipSnapshotCreation: true}
		path, ch, cleanup, err := createSnapshotIfNeeded(cfg, "/dev/orig", 100, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/dev/orig" {
			t.Fatalf("expected original path, got %q", path)
		}
		if ch != nil {
			t.Fatalf("expected nil monitor channel")
		}
		if cleanup == nil {
			t.Fatalf("expected cleanup function")
		}
		cleanup()
	})

	t.Run("creation failure", func(t *testing.T) {
		origCreate := createSnapshot
		defer func() { createSnapshot = origCreate }()
		createSnapshot = func(context.Context, string, string, string) error { return errors.New("fail") }

		cfg := &config.Config{}
		_, _, _, err := createSnapshotIfNeeded(cfg, "/dev/orig", 100, logger)
		if err == nil || !strings.Contains(err.Error(), "snapshot creation failed") {
			t.Fatalf("expected snapshot creation failure, got %v", err)
		}
	})

	t.Run("success with monitor error", func(t *testing.T) {
		origCreate := createSnapshot
		origPath := getSnapshotDevicePath
		origMonitor := monitorSnapshot
		origRemove := removeSnapshot
		defer func() {
			createSnapshot = origCreate
			getSnapshotDevicePath = origPath
			monitorSnapshot = origMonitor
			removeSnapshot = origRemove
		}()

		createSnapshot = func(context.Context, string, string, string) error { return nil }
		getSnapshotDevicePath = func(string, string) string { return "/dev/vg/snap" }
		monitorSnapshot = func(context.Context, string, float64, time.Duration) error { return errors.New("mon") }
		removed := false
		removeSnapshot = func(context.Context, string) error { removed = true; return nil }

		cfg := &config.Config{VolumeGroup: "vg"}
		path, ch, cleanup, err := createSnapshotIfNeeded(cfg, "/dev/vg/orig", 100, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "/dev/vg/snap" {
			t.Fatalf("unexpected snapshot path %q", path)
		}
		if ch == nil {
			t.Fatalf("expected monitor channel")
		}
		select {
		case err := <-ch:
			if err == nil || !strings.Contains(err.Error(), "mon") {
				t.Fatalf("unexpected monitor error: %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("monitor channel did not receive error")
		}
		cleanup()
		if !removed {
			t.Fatalf("expected removeSnapshot to be called")
		}
	})
}

func TestPrepare(t *testing.T) {
	logger := zap.NewNop()

	t.Run("success", func(t *testing.T) {
		origParse := parseSnapshotSize
		origGetVG := getVolumeGroupName
		origGetSize := getVolumeSize
		origSelect := selectVolumeGroupForSize
		origCheck := checkDiskSpace
		origCreate := createSnapshot
		origPath := getSnapshotDevicePath
		origMonitor := monitorSnapshot
		origRemove := removeSnapshot
		defer func() {
			parseSnapshotSize = origParse
			getVolumeGroupName = origGetVG
			getVolumeSize = origGetSize
			selectVolumeGroupForSize = origSelect
			checkDiskSpace = origCheck
			createSnapshot = origCreate
			getSnapshotDevicePath = origPath
			monitorSnapshot = origMonitor
			removeSnapshot = origRemove
		}()

		parseSnapshotSize = func(string, string) (uint64, error) { return 100, nil }
		getVolumeGroupName = func(string) (string, error) { return "vg", nil }
		getVolumeSize = func(string) (uint64, error) { return 50, nil }
		selectVolumeGroupForSize = func(context.Context, []string, uint64) (lvm.VolumeGroup, error) {
			return lvm.VolumeGroup{Name: "target"}, nil
		}
		checkDiskSpace = func(string) (uint64, error) { return 1000, nil }
		createSnapshot = func(context.Context, string, string, string) error { return nil }
		getSnapshotDevicePath = func(string, string) string { return "/dev/vg/snap" }
		monitorSnapshot = func(context.Context, string, float64, time.Duration) error { return nil }
		removed := false
		removeSnapshot = func(context.Context, string) error { removed = true; return nil }

		cfg := &config.Config{TargetVGCandidates: []string{"target"}}
		path, ch, cleanup, err := Prepare(cfg, "/dev/vg/orig", logger)
		if err != nil {
			t.Fatalf("Prepare error: %v", err)
		}
		if path != "/dev/vg/snap" {
			t.Fatalf("unexpected path %q", path)
		}
		if ch == nil {
			t.Fatalf("expected monitor channel")
		}
		cleanup()
		if !removed {
			t.Fatalf("cleanup did not remove snapshot")
		}
	})

	t.Run("calculate error", func(t *testing.T) {
		origParse := parseSnapshotSize
		defer func() { parseSnapshotSize = origParse }()
		parseSnapshotSize = func(string, string) (uint64, error) { return 0, errors.New("bad") }
		cfg := &config.Config{}
		if _, _, _, err := Prepare(cfg, "/dev/vg/orig", logger); err == nil {
			t.Fatalf("expected calculate error")
		}
	})
}
