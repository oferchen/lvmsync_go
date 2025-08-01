// transfer/block.go
package transfer

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"lvmsync_go/config"

	"go.uber.org/zap"
)

// PipeCreationCount tracks the number of pipes created by ReadBlockWithRetries
// when no persistent pipe is supplied. It is mainly used for benchmarking.
var PipeCreationCount int64

func ZeroCopyTransfer(src *os.File, dst *os.File, offset int64, length int64, pipeFds [2]int) error {
	remaining := length
	off := offset
	for remaining > 0 {
		n, err := syscall.Splice(int(src.Fd()), &off, pipeFds[1], nil, int(remaining), 0)
		if err != nil {
			return fmt.Errorf("splice read failed: %v", err)
		}
		if n == 0 {
			break
		}
		_, err = syscall.Splice(pipeFds[0], nil, int(dst.Fd()), nil, int(n), 0)
		if err != nil {
			return fmt.Errorf("splice write failed: %v", err)
		}
		remaining -= int64(n)
	}
	return nil
}

func ReadBlock(src *os.File, offset int64, size int) ([]byte, error) {
	buf := make([]byte, size)
	n, err := src.ReadAt(buf, offset)
	if err != nil {
		return nil, err
	}
	if n != size {
		return nil, fmt.Errorf("short read: expected %d, got %d", size, n)
	}
	return buf, nil
}

func ReadBlockWithRetries(cfg *config.Config, src *os.File, offset int64, useZeroCopy bool, pipeFds [2]int) ([]byte, error) {
	blockSize := cfg.BlockSize
	maxRetries := cfg.MaxRetries
	var data []byte
	var err error

	if useZeroCopy {
		if pipeFds[0] == -1 && pipeFds[1] == -1 {
			if err := syscall.Pipe(pipeFds[:]); err != nil {
				return nil, err
			}
			atomic.AddInt64(&PipeCreationCount, 1)
			defer syscall.Close(pipeFds[0])
			defer syscall.Close(pipeFds[1])
		}

		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		defer r.Close()

		for attempt := 0; attempt < maxRetries; attempt++ {
			err = ZeroCopyTransfer(src, w, offset, int64(blockSize), pipeFds)
			if err == nil {
				break
			}

			Logger.Warn("Zero-copy transfer failed",
				zap.Int64("offset", offset),
				zap.Int("size", blockSize),
				zap.Int("attempt", attempt+1),
				zap.Error(err))

			time.Sleep(100 * time.Millisecond)
		}
		w.Close()

		if err != nil {
			return nil, err
		}

		data, err = io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		if len(data) != blockSize {
			return nil, fmt.Errorf("zero-copy short read: expected %d, got %d", blockSize, len(data))
		}
		return data, nil
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		data, err = ReadBlock(src, offset, blockSize)
		if err == nil {
			return data, nil
		}

		Logger.Warn("Failed to read block",
			zap.Int64("offset", offset),
			zap.Int("size", blockSize),
			zap.Int("attempt", attempt+1),
			zap.Error(err))

		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("failed to read block at offset %d after %d attempts", offset, maxRetries)
}
