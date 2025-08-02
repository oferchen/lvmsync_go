//go:build linux

// transfer/block_linux.go
package transfer

import (
	"fmt"
	"os"
	"syscall"
)

func ZeroCopyTransfer(src *os.File, dst *os.File, offset int64, length int64, pipeFds [2]int) error {
	remaining := length
	off := offset
	for remaining > 0 {
		n, err := syscall.Splice(int(src.Fd()), &off, pipeFds[1], nil, int(remaining), 0)
		if err != nil {
			return fmt.Errorf("splice read failed: %w", err)
		}
		if n == 0 {
			break
		}
		_, err = syscall.Splice(pipeFds[0], nil, int(dst.Fd()), nil, int(n), 0)
		if err != nil {
			return fmt.Errorf("splice write failed: %w", err)
		}
		remaining -= int64(n)
	}
	return nil
}
