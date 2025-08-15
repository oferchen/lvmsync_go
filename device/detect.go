package device

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/lvm"
)

// Detect inspects the path and returns the appropriate Device implementation.
// Regular files return FileDevice, block devices are classified as either LVM
// logical volumes or raw devices based on LVM metadata. When typeHint is not
// empty or "auto", Detect will attempt to open the device using the explicit
// type and will not perform auto detection. logger must be non-nil.
func Detect(ctx context.Context, path string, offline bool, typeHint, fsFreezeCmd, fsThawCmd, lvmEscalation string, freezeTimeout, thawTimeout time.Duration, logger *zap.Logger) (Device, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		logger.Error("device detect failed", zap.String("path", path), zap.String("device_type", "symlink"), zap.Error(err))
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		logger.Error("device detect failed", zap.String("path", resolved), zap.String("device_type", "stat"), zap.Error(err))
		return nil, err
	}
	if typeHint != "" && typeHint != "auto" {
		switch typeHint {
		case "file":
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("expected regular file for type file: %s", resolved)
			}
			dev, err := OpenFile(resolved, logger)
			if err != nil {
				logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "file"), zap.Error(err))
				return nil, err
			}
			logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "file"))
			return dev, nil
		case "lvm":
			if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
				return nil, fmt.Errorf("expected block device for type lvm: %s", resolved)
			}
			cache := lvm.NewDeviceFDCache(logger)
			defer cache.Close()
			dev, err := OpenLVM(resolved, cache, lvmEscalation, logger)
			if err != nil {
				logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "lvm"), zap.Error(err))
				return nil, err
			}
			logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "lvm"))
			return dev, nil
		case "raw":
			if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
				return nil, fmt.Errorf("expected block device for type raw: %s", resolved)
			}
			var freezePath, thawPath string
			var freezeArgs, thawArgs []string
			if fsFreezeCmd != "" {
				parts := strings.Fields(fsFreezeCmd)
				if len(parts) > 0 {
					freezePath = parts[0]
					freezeArgs = parts[1:]
				}
			}
			if fsThawCmd != "" {
				parts := strings.Fields(fsThawCmd)
				if len(parts) > 0 {
					thawPath = parts[0]
					thawArgs = parts[1:]
				}
			}
			dev, err := OpenRaw(ctx, resolved, offline, freezePath, freezeArgs, thawPath, thawArgs, freezeTimeout, thawTimeout, logger)
			if err != nil {
				logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "raw"), zap.Error(err))
				return nil, err
			}
			logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "raw"))
			return dev, nil
		default:
			return nil, fmt.Errorf("unknown device type %q", typeHint)
		}
	}
	if info.Mode().IsRegular() {
		dev, err := OpenFile(resolved, logger)
		if err != nil {
			logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "file"), zap.Error(err))
			return nil, err
		}
		logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "file"))
		return dev, nil
	}
	if info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice == 0 {
		if _, err := lvm.GetVolumeGroupName(resolved); err == nil {
			cache := lvm.NewDeviceFDCache(logger)
			defer cache.Close()
			dev, err := OpenLVM(resolved, cache, lvmEscalation, logger)
			if err != nil {
				logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "lvm"), zap.Error(err))
				return nil, err
			}
			logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "lvm"))
			return dev, nil
		} else {
			logger.Debug("detect device failed", zap.String("path", resolved), zap.String("device_type", "lvm"), zap.Error(err))
		}
		var freezePath, thawPath string
		var freezeArgs, thawArgs []string
		if fsFreezeCmd != "" {
			parts := strings.Fields(fsFreezeCmd)
			if len(parts) > 0 {
				freezePath = parts[0]
				freezeArgs = parts[1:]
			}
		}
		if fsThawCmd != "" {
			parts := strings.Fields(fsThawCmd)
			if len(parts) > 0 {
				thawPath = parts[0]
				thawArgs = parts[1:]
			}
		}
		dev, err := OpenRaw(ctx, resolved, offline, freezePath, freezeArgs, thawPath, thawArgs, freezeTimeout, thawTimeout, logger)
		if err != nil {
			logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "raw"), zap.Error(err))
			return nil, err
		}
		logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "raw"))
		return dev, nil
	}
	err = fmt.Errorf("unsupported path type: %s", resolved)
	logger.Error("device detect failed", zap.String("path", resolved), zap.String("device_type", "unknown"), zap.Error(err))
	return nil, err
}
