// Package device includes a helper to configure kernel readahead hints for
// block devices.
package device

import (
	"os"

	"golang.org/x/sys/unix"
)

// SetReadAhead adjusts the kernel readahead window for the provided file. The
// size is specified in bytes and rounded down to a 512-byte sector. On systems
// where the ioctl is unsupported or the file is not a block device, the call
// succeeds with a nil error.
func SetReadAhead(f *os.File, size int) error {
	if size <= 0 {
		return nil
	}
	sectors := size / 512
	if sectors == 0 {
		sectors = 1
	}
	if err := unix.IoctlSetInt(int(f.Fd()), unix.BLKRASET, sectors); err != nil {
		if err == unix.ENOTTY || err == unix.EOPNOTSUPP || err == unix.EINVAL {
			return nil
		}
		return err
	}
	return nil
}
