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

func DetectSectorSize(_ *os.File) (int, error) { return 512, nil }

func getAlignedBlockBuffer(size int) []byte { return getBlockBuffer(size) }

func putAlignedBlockBuffer(buf []byte) { putBlockBuffer(buf) }

func punchHole(f *os.File, offset uint64, length int) error {
	zero := make([]byte, length)
	_, err := f.WriteAt(zero, int64(offset))
	return err
}

func fdatasync(f *os.File) error { return f.Sync() }

func openFileODirect(path string, flag int) (*os.File, bool, error) {
	f, err := os.OpenFile(path, flag, 0)
	return f, false, err
}

func seekHoleSupported(_ *os.File) bool { return false }

func nextDataOffset(_ *os.File, _ int64) (int64, error) { return -1, nil }
