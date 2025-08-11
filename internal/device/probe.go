// Package device exposes helpers to determine logical and physical block sizes
// for an opened file descriptor. Regular files default to 4096 bytes when the
// ioctl is unsupported.
package device

import (
	"os"

	"golang.org/x/sys/unix"
)

// BlockSizes returns logical and physical block sizes in bytes.
func BlockSizes(f *os.File) (logical, physical int, err error) {
	l, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKSSZGET)
	if err != nil {
		l = 4096
	}
	p, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKPBSZGET)
	if err != nil {
		p = l
	}
	return l, p, nil
}
