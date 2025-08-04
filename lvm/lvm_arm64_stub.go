//go:build arm64

package lvm

func SelectVolumeGroupByFreeSpace(_ []string) (string, int, error) {
	return "", 0, nil
}

func GetVolumeGroupFreeSpace(_ string) (int, error) {
	return 0, nil
}
