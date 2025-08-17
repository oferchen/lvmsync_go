//go:build linux
// +build linux

package transfer

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

func ZeroCopyTransfer(src, dst *os.File, offset, length int64, pipeFds [2]int) error {
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

// DetectSectorSize returns the logical sector size for the given file descriptor.
func DetectSectorSize(f *os.File) (int, error) {
	size, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKSSZGET)
	if err != nil {
		var st unix.Stat_t
		if statErr := unix.Fstat(int(f.Fd()), &st); statErr != nil {
			return 0, fmt.Errorf("detect sector size: %w", err)
		}
		size = int(st.Blksize)
	}
	return size, nil
}

var alignedPools sync.Map

func getAlignedBlockBuffer(size int) []byte {
	if p, ok := alignedPools.Load(size); ok {
		if pool, ok := p.(*sync.Pool); ok {
			if bufAny := pool.Get(); bufAny != nil {
				if bp, ok := bufAny.(*[]byte); ok {
					return *bp
				}
			}
		}
	}
	pool := &sync.Pool{New: func() any {
		b, err := unix.Mmap(-1, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
		if err != nil {
			buf := make([]byte, size)
			return &buf
		}
		return &b
	}}
	actual, _ := alignedPools.LoadOrStore(size, pool)
	bufAny := actual.(*sync.Pool).Get()
	if bp, ok := bufAny.(*[]byte); ok {
		return *bp
	}
	if b, ok := bufAny.([]byte); ok {
		return b
	}
	buf := make([]byte, size)
	return buf
}

func putAlignedBlockBuffer(buf []byte) {
	if p, ok := alignedPools.Load(len(buf)); ok {
		if pool, ok := p.(*sync.Pool); ok {
			pool.Put(&buf)
		}
	}
}

func punchHole(f *os.File, offset uint64, length int) error {
	return unix.Fallocate(int(f.Fd()), unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, int64(offset), int64(length))
}

func fdatasync(f *os.File) error {
	return unix.Fdatasync(int(f.Fd()))
}

func openFileODirect(path string, flag int) (*os.File, bool, error) {
	fd, err := unix.Open(path, flag|unix.O_DIRECT, 0)
	if err != nil {
		f, err2 := os.OpenFile(path, flag, 0)
		return f, false, err2
	}
	return os.NewFile(uintptr(fd), path), true, nil
}
