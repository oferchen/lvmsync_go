package client

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	"lvmsync_go/lvm"
)

// Runner provides snapshot preparation helpers with injectable dependencies.
type Runner struct {
	parseSnapshotSize        func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error)
	getVolumeGroupName       func(string) (string, error)
	getVolumeSize            func(string, *lvm.FDCache, *zap.Logger) (uint64, error)
	selectVolumeGroupForSize func(context.Context, []string, uint64) (lvm.VolumeGroup, error)
	checkDiskSpace           func(string, *zap.Logger) (uint64, error)
	createSnapshot           func(context.Context, string, string, string, *zap.Logger) error
	getSnapshotDevicePath    func(string, string, *zap.Logger) string
	monitorSnapshot          func(context.Context, string, float64, time.Duration, *zap.Logger) error
	removeSnapshot           func(context.Context, string, *zap.Logger) error
	snapshotName             func() string
}

// NewRunner constructs a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{
		parseSnapshotSize:        lvm.ParseSnapshotSize,
		getVolumeGroupName:       lvm.GetVolumeGroupName,
		getVolumeSize:            lvm.GetVolumeSize,
		selectVolumeGroupForSize: lvm.SelectVolumeGroupForSize,
		checkDiskSpace:           lvm.CheckDiskSpace,
		createSnapshot:           lvm.CreateSnapshot,
		getSnapshotDevicePath:    lvm.GetSnapshotDevicePath,
		monitorSnapshot:          lvm.MonitorSnapshot,
		removeSnapshot:           lvm.RemoveSnapshot,
		snapshotName:             func() string { return fmt.Sprintf("snap-%d", time.Now().Unix()) },
	}
}

// NewRunnerWithDeps constructs a Runner overriding dependencies. Nil functions use defaults.
func NewRunnerWithDeps(
	parse func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error),
	vgName func(string) (string, error),
	volSize func(string, *lvm.FDCache, *zap.Logger) (uint64, error),
	selectVG func(context.Context, []string, uint64) (lvm.VolumeGroup, error),
	diskSpace func(string, *zap.Logger) (uint64, error),
	create func(context.Context, string, string, string, *zap.Logger) error,
	snapPath func(string, string, *zap.Logger) string,
	monitor func(context.Context, string, float64, time.Duration, *zap.Logger) error,
	remove func(context.Context, string, *zap.Logger) error,
	snapName func() string,
) *Runner {
	r := NewRunner()
	if parse != nil {
		r.parseSnapshotSize = parse
	}
	if vgName != nil {
		r.getVolumeGroupName = vgName
	}
	if volSize != nil {
		r.getVolumeSize = volSize
	}
	if selectVG != nil {
		r.selectVolumeGroupForSize = selectVG
	}
	if diskSpace != nil {
		r.checkDiskSpace = diskSpace
	}
	if create != nil {
		r.createSnapshot = create
	}
	if snapPath != nil {
		r.getSnapshotDevicePath = snapPath
	}
	if monitor != nil {
		r.monitorSnapshot = monitor
	}
	if remove != nil {
		r.removeSnapshot = remove
	}
	if snapName != nil {
		r.snapshotName = snapName
	}
	return r
}

func (r *Runner) calculateSnapshotSize(cfg *config.Config, originalVolume string, cache *lvm.FDCache, logger *zap.Logger) (uint64, error) {
	snapshotBytes, err := r.parseSnapshotSize(cfg.SnapshotSize, originalVolume, cache, logger)
	if err != nil {
		return 0, fmt.Errorf("failed to parse snapshot size: %w", err)
	}
	return snapshotBytes, nil
}

func (r *Runner) ensureVolumeGroups(ctx context.Context, cfg *config.Config, originalVolume string, cache *lvm.FDCache, logger *zap.Logger) error {
	if cfg.VolumeGroup == "" {
		vg, err := r.getVolumeGroupName(originalVolume)
		if err != nil {
			return fmt.Errorf("failed to determine source volume group: %w", err)
		}
		cfg.VolumeGroup = vg
		logger.Info("Using source volume group", zap.String("volume_group", cfg.VolumeGroup))
	}

	if cfg.TargetVolumeGroup == "" && len(cfg.TargetVGCandidates) > 0 {
		lvSize, err := r.getVolumeSize(originalVolume, cache, logger)
		if err != nil {
			return fmt.Errorf("failed to determine volume size: %w", err)
		}
		vg, err := r.selectVolumeGroupForSize(ctx, cfg.TargetVGCandidates, lvSize)
		if err != nil {
			return fmt.Errorf("failed to select target volume group: %w", err)
		}
		cfg.TargetVolumeGroup = vg.Name
		logger.Info("Selected target volume group", zap.String("target_volume_group", cfg.TargetVolumeGroup))
	}
	return nil
}

func (r *Runner) checkDiskSpaceForSnapshot(cfg *config.Config, snapshotBytes uint64, logger *zap.Logger) error {
	if cfg.SkipDiskCheck {
		return nil
	}
	freeSpace, err := r.checkDiskSpace("/", logger)
	if err != nil {
		return fmt.Errorf("disk space check failed: %w", err)
	}
	if freeSpace < snapshotBytes {
		return fmt.Errorf("insufficient disk space for snapshot: free %d required %d", freeSpace, snapshotBytes)
	}
	logger.Info("Disk space check passed", zap.Uint64("free_bytes", freeSpace))
	return nil
}

func (r *Runner) createSnapshotIfNeeded(ctx context.Context, cfg *config.Config, originalVolume string, snapshotBytes uint64, logger *zap.Logger) (string, chan error, func(), error) {
	snapshotPath := originalVolume
	var monitorErrCh chan error
	cleanup := func() {}

	if cfg.SkipSnapshotCreation {
		if !cfg.Force {
			return "", nil, nil, fmt.Errorf("skip snapshot creation requires --force")
		}
		return snapshotPath, monitorErrCh, cleanup, nil
	}

	monitorErrCh = make(chan error, 1)
	snapshotName := r.snapshotName()
	if err := r.createSnapshot(ctx, originalVolume, snapshotName, strconv.FormatUint(snapshotBytes, 10), logger); err != nil {
		return "", nil, nil, fmt.Errorf("snapshot creation failed: %w", err)
	}
	snapshotPath = r.getSnapshotDevicePath(snapshotName, cfg.VolumeGroup, logger)
	lvm.RegisterSnapshot(snapshotPath, logger)
	logger.Info("Snapshot created", zap.String("snapshot", snapshotPath))

	monitorCtx, cancel := context.WithCancel(ctx)
	mon := r.monitorSnapshot
	go func() {
		defer close(monitorErrCh)
		if err := mon(monitorCtx, snapshotPath, 80.0, 10*time.Second, logger); err != nil && err != context.Canceled {
			logger.Error("Snapshot monitor error", zap.Error(err))
			select {
			case monitorErrCh <- err:
			default:
			}
		}
	}()

	cleanup = func() {
		cancel()
		lvm.UnregisterSnapshot(snapshotPath)
		removeCtx, removeCancel := context.WithTimeout(ctx, 10*time.Second)
		defer removeCancel()
		if err := r.removeSnapshot(removeCtx, snapshotPath, logger); err != nil {
			logger.Warn("Failed to remove snapshot", zap.Error(err))
		} else {
			logger.Info("Snapshot removed", zap.String("snapshot", snapshotPath))
		}
	}

	return snapshotPath, monitorErrCh, cleanup, nil
}

// PrepareSnapshot sets up a snapshot for the given original volume.
// It returns the snapshot path, an optional monitor error channel,
// a cleanup function, and any error encountered.
func (r *Runner) PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	cache, err := lvm.NewDeviceFDCache(logger)
	if err != nil {
		return "", nil, nil, err
	}
	defer cache.Close()

	snapshotBytes, err := r.calculateSnapshotSize(cfg, originalVolume, cache, logger)
	if err != nil {
		return "", nil, nil, err
	}

	if err := r.ensureVolumeGroups(ctx, cfg, originalVolume, cache, logger); err != nil {
		return "", nil, nil, err
	}

	if err := r.checkDiskSpaceForSnapshot(cfg, snapshotBytes, logger); err != nil {
		return "", nil, nil, err
	}

	return r.createSnapshotIfNeeded(ctx, cfg, originalVolume, snapshotBytes, logger)
}

var defaultRunner = NewRunner()

// PrepareSnapshot wraps the default runner's PrepareSnapshot.
func PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return defaultRunner.PrepareSnapshot(ctx, cfg, originalVolume, logger)
}
