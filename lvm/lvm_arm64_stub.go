//go:build arm64

package lvm

func SelectVolumeGroupByFreeSpace(_ []string) (VolumeGroup, error) {
	return VolumeGroup{}, nil
}

func GetVolumeGroupFreeSpace(_ string) (uint64, error) {
	return 0, nil
}
