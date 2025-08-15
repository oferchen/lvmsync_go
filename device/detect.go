package device

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"lvmsync_go/lvm"
)

// Detect inspects the path and returns the appropriate Device implementation.
// Regular files return FileDevice, block devices are classified as either LVM
// logical volumes or raw devices based on LVM metadata.
func Detect(ctx context.Context, path string, offline bool, fsFreezeCmd, fsThawCmd string, logger *zap.Logger) (Device, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if logger != nil {
			logger.Error("device detect failed", zap.String("path", path), zap.String("device_type", "symlink"), zap.Error(err))
		}
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if logger != nil {
			logger.Error("device detect failed", zap.String("path", resolved), zap.String("device_type", "stat"), zap.Error(err))
		}
		return nil, err
	}
	if info.Mode().IsRegular() {
		dev, err := OpenFile(resolved, logger)
		if err != nil {
			if logger != nil {
				logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "file"), zap.Error(err))
			}
			return nil, err
		}
		if logger != nil {
			logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "file"))
		}
		return dev, nil
	}
	if info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice == 0 {
		if _, err := lvm.GetVolumeGroupName(resolved); err == nil {
			dev, err := OpenLVM(resolved, logger)
			if err != nil {
				if logger != nil {
					logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "lvm"), zap.Error(err))
				}
				return nil, err
			}
			if logger != nil {
				logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "lvm"))
			}
			return dev, nil
		} else {
			if logger != nil {
				logger.Debug("detect device failed", zap.String("path", resolved), zap.String("device_type", "lvm"), zap.Error(err))
			}
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
		dev, err := OpenRaw(ctx, resolved, offline, freezePath, freezeArgs, thawPath, thawArgs, logger)
		if err != nil {
			if logger != nil {
				logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "raw"), zap.Error(err))
			}
			return nil, err
		}
		if logger != nil {
			logger.Info("detect device success", zap.String("path", resolved), zap.String("device_type", "raw"))
		}
		return dev, nil
	}
	err = fmt.Errorf("unsupported path type: %s", resolved)
	if logger != nil {
		logger.Error("detect device failed", zap.String("path", resolved), zap.String("device_type", "unknown"), zap.Error(err))
	}
	return nil, err
}
