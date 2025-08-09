//go:build linux
// +build linux

package transfer

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func ZeroCopyTransfer(src *os.File, dst *os.File, offset int64, length int64, pipeFds [2]int) error {
	remaining := length
	off := offset
	for remaining > 0 {
		n, err := unix.Splice(int(src.Fd()), &off, pipeFds[1], nil, int(remaining), 0)
		if err != nil {
			return fmt.Errorf("splice read failed: %w", err)
		}
		if n == 0 {
			break
		}
		_, err = unix.Splice(pipeFds[0], nil, int(dst.Fd()), nil, int(n), 0)
		if err != nil {
			return fmt.Errorf("splice write failed: %w", err)
		}
		remaining -= n
	}
	return nil
}
