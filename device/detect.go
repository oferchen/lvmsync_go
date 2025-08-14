package device

import (
	"fmt"
	"os"

	"lvmsync_go/lvm"
)

// Detect inspects the path and returns the appropriate Device implementation.
// Regular files return FileDevice, block devices are classified as either LVM
// logical volumes or raw devices based on LVM metadata.
func Detect(path string, offline bool, fsFreezeCmd string) (Device, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().IsRegular() {
		return OpenFile(path)
	}
	if info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice == 0 {
		if _, err := lvm.GetVolumeGroupName(path); err == nil {
			return OpenLVM(path)
		}
		return OpenRaw(path, offline, fsFreezeCmd)
	}
	return nil, fmt.Errorf("unsupported path type: %s", path)
}
