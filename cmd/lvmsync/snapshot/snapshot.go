package snapshot

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/lvm"
)

var (
	parseSnapshotSize        = lvm.ParseSnapshotSize
	getVolumeGroupName       = lvm.GetVolumeGroupName
	getVolumeSize            = lvm.GetVolumeSize
	selectVolumeGroupForSize = lvm.SelectVolumeGroupForSize
	checkDiskSpace           = lvm.CheckDiskSpace
	createSnapshot           = lvm.CreateSnapshot
	getSnapshotDevicePath    = lvm.GetSnapshotDevicePath
	monitorSnapshot          = lvm.MonitorSnapshot
	removeSnapshot           = lvm.RemoveSnapshot
)

// Prepare sets up a snapshot for the given original volume. It returns the snapshot path,
// an optional monitor error channel, a cleanup function, and any error encountered.
func Prepare(cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	snapshotBytes, err := calculateSnapshotSize(cfg, originalVolume)
	if err != nil {
		return "", nil, nil, err
	}

	if err := ensureVolumeGroups(cfg, originalVolume, logger); err != nil {
		return "", nil, nil, err
	}

	if err := checkDiskSpaceForSnapshot(cfg, snapshotBytes, logger); err != nil {
		return "", nil, nil, err
	}

	return createSnapshotIfNeeded(cfg, originalVolume, snapshotBytes, logger)
}

func calculateSnapshotSize(cfg *config.Config, originalVolume string) (uint64, error) {
	snapshotBytes, err := parseSnapshotSize(cfg.SnapshotSize, originalVolume)
	if err != nil {
		return 0, fmt.Errorf("failed to parse snapshot size: %w", err)
	}
	return snapshotBytes, nil
}

func ensureVolumeGroups(cfg *config.Config, originalVolume string, logger *zap.Logger) error {
	if cfg.VolumeGroup == "" {
		vg, err := getVolumeGroupName(originalVolume)
		if err != nil {
			return fmt.Errorf("failed to determine source volume group: %w", err)
		}
		cfg.VolumeGroup = vg
		logger.Info("Using source volume group", zap.String("volume_group", cfg.VolumeGroup))
	}

	if cfg.TargetVolumeGroup == "" && len(cfg.TargetVGCandidates) > 0 {
		lvSize, err := getVolumeSize(originalVolume)
		if err != nil {
			return fmt.Errorf("failed to determine volume size: %w", err)
		}
		vg, err := selectVolumeGroupForSize(context.Background(), cfg.TargetVGCandidates, lvSize)
		if err != nil {
			return fmt.Errorf("failed to select target volume group: %w", err)
		}
		cfg.TargetVolumeGroup = vg.Name
		logger.Info("Selected target volume group", zap.String("target_volume_group", cfg.TargetVolumeGroup))
	}
	return nil
}

func checkDiskSpaceForSnapshot(cfg *config.Config, snapshotBytes uint64, logger *zap.Logger) error {
	if cfg.SkipDiskCheck {
		return nil
	}
	freeSpace, err := checkDiskSpace("/")
	if err != nil {
		return fmt.Errorf("disk space check failed: %w", err)
	}
	if freeSpace < snapshotBytes {
		return fmt.Errorf("insufficient disk space for snapshot: free %d required %d", freeSpace, snapshotBytes)
	}
	logger.Info("Disk space check passed", zap.Uint64("free", freeSpace))
	return nil
}

func createSnapshotIfNeeded(cfg *config.Config, originalVolume string, snapshotBytes uint64, logger *zap.Logger) (string, chan error, func(), error) {
	snapshotPath := originalVolume
	var monitorErrCh chan error
	cleanup := func() {}

	if cfg.SkipSnapshotCreation {
		return snapshotPath, monitorErrCh, cleanup, nil
	}

	monitorErrCh = make(chan error, 1)
	snapshotName := fmt.Sprintf("snap-%d", time.Now().Unix())
	if err := createSnapshot(context.Background(), originalVolume, snapshotName, strconv.FormatUint(snapshotBytes, 10)); err != nil {
		return "", nil, nil, fmt.Errorf("snapshot creation failed: %w", err)
	}
	snapshotPath = getSnapshotDevicePath(snapshotName, cfg.VolumeGroup)
	logger.Info("Snapshot created", zap.String("snapshot", snapshotPath))

	monitorCtx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := monitorSnapshot(monitorCtx, snapshotPath, 80.0, 10*time.Second); err != nil && err != context.Canceled {
			zap.L().Error("Snapshot monitor error", zap.Error(err))
			monitorErrCh <- err
		}
	}()

	cleanup = func() {
		cancel()
		if err := removeSnapshot(context.Background(), snapshotPath); err != nil {
			logger.Warn("Failed to remove snapshot", zap.Error(err))
		} else {
			logger.Info("Snapshot removed", zap.String("snapshot", snapshotPath))
		}
	}

	return snapshotPath, monitorErrCh, cleanup, nil
}
