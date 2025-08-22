package device

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kballard/go-shellquote"
	"go.uber.org/zap"

	"lvmsync_go/internal/privilege"
	"lvmsync_go/lvm"
)

func verifyPartition(ctx context.Context, dev Device, runner *Runner, logger *zap.Logger) error {
	sig := partitionSignaturesFromContext(ctx)
	if sig == nil || (sig.gpt == "" && sig.mbr == "" && sig.layout == nil) {
		return nil
	}
	id, err := dev.Identity(ctx)
	if err != nil {
		return err
	}
	layout, err := readPartitionLayout(ctx, dev.Path(), runner)
	if err != nil {
		return err
	}
	if sig.gpt != "" && id.GPTUUID != sig.gpt {
		return fmt.Errorf("precondition: %w", ErrPartitionMismatch)
	}
	if sig.mbr != "" && id.MBRSignature != sig.mbr {
		return fmt.Errorf("precondition: %w", ErrPartitionMismatch)
	}
	if diffs := diffPartitionLayouts(sig.layout, layout); len(diffs) > 0 {
		logger.Error("partition_layout_mismatch", zap.Any("diff", diffs))
		return fmt.Errorf("precondition: %w", ErrPartitionMismatch)
	}
	return nil
}

// detectFileDevice opens a regular file as a device.
func detectFileDevice(path string, logger *zap.Logger) (Device, error) {
	dev, err := OpenFile(path, logger)
	if err != nil {
		logger.Error("detect_device_failed", zap.String("path", path), zap.String("device_type", TypeFile), zap.Error(err))
		return nil, err
	}
	logger.Info("detect_device_success", zap.String("path", path), zap.String("device_type", TypeFile))
	return dev, nil
}

// detectLVMDevice opens an LVM logical volume.
func detectLVMDevice(ctx context.Context, path, lvmEscalation string, runner *Runner, logger *zap.Logger) (Device, error) {
	if err := lvm.VerifyEscalationCommand(lvmEscalation); err != nil {
		logger.Error("detect_device_failed", zap.String("path", path), zap.String("device_type", TypeLVM), zap.Error(err))
		return nil, err
	}
	cache, err := lvm.NewDeviceFDCache(logger)
	if err != nil {
		return nil, err
	}
	defer cache.Close()
	dev, err := runner.OpenLVM(ctx, path, cache, lvmEscalation, logger)
	if err != nil {
		logger.Error("detect_device_failed", zap.String("path", path), zap.String("device_type", TypeLVM), zap.Error(err))
		return nil, err
	}
	if err := verifyPartition(ctx, dev, runner, logger); err != nil {
		dev.Close()
		logger.Error("detect_device_failed", zap.String("path", path), zap.String("device_type", TypeLVM), zap.Error(err))
		return nil, err
	}
	logger.Info("detect_device_success", zap.String("path", path), zap.String("device_type", TypeLVM))
	return dev, nil
}

// detectRawDevice opens a raw block device.
func detectRawDevice(
	ctx context.Context,
	path string,
	offline bool,
	fsFreezeCmd, fsThawCmd string,
	freezeTimeout, thawTimeout time.Duration,
	esc privilege.Escalator,
	logger *zap.Logger,
	runner *Runner,
) (Device, error) {
	var freezePath, thawPath string
	var freezeArgs, thawArgs []string
	if fsFreezeCmd != "" {
		parts, err := shellquote.Split(fsFreezeCmd)
		if err != nil {
			err = fmt.Errorf("invalid freeze command: %w", err)
			logger.Error("detect_device_failed", zap.String("path", path), zap.String("device_type", TypeRaw), zap.Error(err))
			return nil, err
		}
		if len(parts) > 0 {
			freezePath = parts[0]
			freezeArgs = parts[1:]
		}
	}
	if fsThawCmd != "" {
		parts, err := shellquote.Split(fsThawCmd)
		if err != nil {
			err = fmt.Errorf("invalid thaw command: %w", err)
			logger.Error("detect_device_failed", zap.String("path", path), zap.String("device_type", TypeRaw), zap.Error(err))
			return nil, err
		}
		if len(parts) > 0 {
			thawPath = parts[0]
			thawArgs = parts[1:]
		}
	}
	dev, err := runner.OpenRaw(ctx, path, offline, freezePath, freezeArgs, thawPath, thawArgs, freezeTimeout, thawTimeout, esc, logger)
	if err != nil {
		logger.Error("detect_device_failed", zap.String("path", path), zap.String("device_type", TypeRaw), zap.Error(err))
		return nil, err
	}
	if err := verifyPartition(ctx, dev, runner, logger); err != nil {
		dev.Close()
		logger.Error("detect_device_failed", zap.String("path", path), zap.String("device_type", TypeRaw), zap.Error(err))
		return nil, err
	}
	logger.Info("detect_device_success", zap.String("path", path), zap.String("device_type", TypeRaw))
	return dev, nil
}

// Detect inspects the path and returns the appropriate Device implementation.
// Regular files return FileDevice, block devices are classified as either LVM
// logical volumes or raw devices based on LVM metadata. When explicitType is
// set via --source-type or --dest-type and is not TypeAuto, Detect will attempt
// to open the device using the explicit type without auto detection. logger
// must be non-nil.
func Detect(
	ctx context.Context,
	path string,
	offline bool,
	explicitType, fsFreezeCmd, fsThawCmd, lvmEscalation string,
	freezeTimeout, thawTimeout time.Duration,
	esc privilege.Escalator,
	logger *zap.Logger,
	runner *Runner,
) (Device, error) {
	if runner == nil {
		runner = NewRunner()
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		logger.Error("device_detect_failed", zap.String("path", path), zap.String("device_type", "symlink"), zap.Error(err))
		return nil, err
	}
	if !filepath.IsAbs(resolved) {
		err := fmt.Errorf("precondition: path must be absolute: %s", path)
		logger.Error("device_detect_failed", zap.String("path", path), zap.String("device_type", "relative"), zap.Error(err))
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		logger.Error("device_detect_failed", zap.String("path", resolved), zap.String("device_type", "stat"), zap.Error(err))
		return nil, err
	}
	if sig := partitionSignaturesFromContext(ctx); sig != nil {
		gpt, mbr, err := readPartitionSignatures(resolved)
		if err != nil {
			logger.Error("device_detect_failed", zap.String("path", resolved), zap.String("device_type", "partition"), zap.Error(err))
			return nil, err
		}
		var layout []partition
		if info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice == 0 {
			layout, err = readPartitionLayout(ctx, resolved, runner)
			if err != nil {
				logger.Error("device_detect_failed", zap.String("path", resolved), zap.String("device_type", "partition"), zap.Error(err))
				return nil, err
			}
		}
		if sig.gpt == "" && sig.mbr == "" && sig.layout == nil {
			if gpt == "" && mbr == "" {
				err := fmt.Errorf("precondition: partition signatures missing")
				logger.Error("device_detect_failed", zap.String("path", resolved), zap.String("device_type", "partition"), zap.Error(err))
				return nil, err
			}
			sig.gpt = gpt
			sig.mbr = mbr
			sig.layout = layout
		} else {
			if gpt == "" && mbr == "" ||
				(sig.gpt != "" && gpt != sig.gpt) ||
				(sig.mbr != "" && mbr != sig.mbr) {
				err := fmt.Errorf("precondition: %w", ErrPartitionMismatch)
				logger.Error("device_detect_failed", zap.String("path", resolved), zap.String("device_type", "partition"), zap.Error(err))
				return nil, err
			}
			if diffs := diffPartitionLayouts(sig.layout, layout); len(diffs) > 0 {
				err := fmt.Errorf("precondition: %w", ErrPartitionMismatch)
				logger.Error("device_detect_failed", zap.String("path", resolved), zap.String("device_type", "partition"), zap.Any("diff", diffs), zap.Error(err))
				return nil, err
			}
		}
	}
	if explicitType != "" && explicitType != TypeAuto {
		switch explicitType {
		case TypeFile:
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("expected regular file for type file: %s", resolved)
			}
			return detectFileDevice(resolved, logger)
		case TypeLVM:
			if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
				return nil, fmt.Errorf("expected block device for type lvm: %s", resolved)
			}
			return detectLVMDevice(ctx, resolved, lvmEscalation, runner, logger)
		case TypeRaw:
			if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
				return nil, fmt.Errorf("expected block device for type raw: %s", resolved)
			}
			return detectRawDevice(ctx, resolved, offline, fsFreezeCmd, fsThawCmd, freezeTimeout, thawTimeout, esc, logger, runner)
		default:
			return nil, fmt.Errorf("unknown device type %q", explicitType)
		}
	}
	if info.Mode().IsRegular() {
		return detectFileDevice(resolved, logger)
	}
	if info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice == 0 {
		out, err := runner.command.CommandContext(ctx, "blkid", "-o", "value", "-s", "TYPE", resolved).Output()
		fsType := strings.TrimSpace(string(out))
		if err == nil && fsType == "LVM2_member" {
			return detectLVMDevice(ctx, resolved, lvmEscalation, runner, logger)
		}
		return detectRawDevice(ctx, resolved, offline, fsFreezeCmd, fsThawCmd, freezeTimeout, thawTimeout, esc, logger, runner)
	}
	err = fmt.Errorf("unsupported path type: %s", resolved)
	logger.Error("device_detect_failed", zap.String("path", resolved), zap.String("device_type", "unknown"), zap.Error(err))
	return nil, err
}
