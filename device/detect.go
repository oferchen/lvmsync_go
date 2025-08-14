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
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if info.Mode().IsRegular() {
		return OpenFile(resolved)
	}
	if info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice == 0 {
		if _, err := lvm.GetVolumeGroupName(resolved); err == nil {
			return OpenLVM(resolved)
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
		return OpenRaw(ctx, resolved, offline, freezePath, freezeArgs, thawPath, thawArgs, logger)
	}
	return nil, fmt.Errorf("unsupported path type: %s", resolved)
}
