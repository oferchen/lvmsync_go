//go:build !linux
// +build !linux

package transfer

import (
	"fmt"
	"io"
	"os"
)

func ZeroCopyTransfer(src *os.File, dst *os.File, offset int64, length int64, pipeFds [2]int) error {
	if _, err := src.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}
	if _, err := io.CopyN(dst, src, length); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}
	return nil
}
