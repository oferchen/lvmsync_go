package device

import (
	"fmt"
	"os"
	"path/filepath"

	"lvmsync_go/lvm"
)

// Detect inspects the path and returns the appropriate Device implementation.
// Regular files return FileDevice, block devices are classified as either LVM
// logical volumes or raw devices based on LVM metadata.
func Detect(path string, offline bool, fsFreezeCmd, fsThawCmd string) (Device, error) {
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
		return OpenRaw(resolved, offline, fsFreezeCmd, fsThawCmd)
	}
	return nil, fmt.Errorf("unsupported path type: %s", resolved)
}
