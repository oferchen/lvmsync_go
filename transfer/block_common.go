package transfer

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

// PipeCreationCount tracks the number of pipes created by ReadBlockWithRetries
// when no persistent pipe is supplied. It is mainly used for benchmarking.
var PipeCreationCount int64

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

//nolint:revive // high complexity is acceptable for this low-level function
func ReadBlockWithRetries(cfg *config.Config, src *os.File, offset int64, useZeroCopy bool, pipeFds [2]int, logger *zap.Logger) ([]byte, error) {
	if useZeroCopy {
		return readWithZeroCopy(cfg, src, offset, pipeFds, logger)
	}
	return retryRead(cfg, src, offset, logger)
}

//revive:disable-next-line:cognitive-complexity
func readWithZeroCopy(cfg *config.Config, src *os.File, offset int64, pipeFds [2]int, logger *zap.Logger) ([]byte, error) {
	blockSize := cfg.BlockSize
	maxRetries := cfg.MaxRetries

	if pipeFds[0] == -1 && pipeFds[1] == -1 {
		if err := syscall.Pipe(pipeFds[:]); err != nil {
			return nil, err
		}
		atomic.AddInt64(&PipeCreationCount, 1)
		defer func() {
			if closeErr := syscall.Close(pipeFds[0]); closeErr != nil {
				if logger != nil {
					logger.Warn("close pipe", zap.Int("fd", pipeFds[0]), zap.Error(closeErr))
				}
			}
		}()
		defer func() {
			if closeErr := syscall.Close(pipeFds[1]); closeErr != nil {
				if logger != nil {
					logger.Warn("close pipe", zap.Int("fd", pipeFds[1]), zap.Error(closeErr))
				}
			}
		}()
	}

	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := r.Close(); closeErr != nil {
			if logger != nil {
				logger.Warn("pipe read close", zap.Error(closeErr))
			}
		}
	}()

	for attempt := 0; attempt < maxRetries; attempt++ {
		err = ZeroCopyTransfer(src, w, offset, int64(blockSize), pipeFds)
		if err == nil {
			break
		}

		if logger != nil {
			logger.Warn("Zero-copy transfer failed",
				zap.Int64("offset", offset),
				zap.Int("size_bytes", blockSize),
				zap.Int("attempt", attempt+1),
				zap.Error(err))
		}

		time.Sleep(100 * time.Millisecond)
	}
	if errClose := w.Close(); errClose != nil {
		return nil, errClose
	}
	if err != nil {
		return nil, err
	}

	var data []byte
	if cfg.ODirect {
		data = getAlignedBlockBuffer(blockSize)
	} else {
		data = getBlockBuffer(blockSize)
	}
	_, err = io.ReadFull(r, data)
	if err != nil {
		if cfg.ODirect {
			putAlignedBlockBuffer(data)
		} else {
			putBlockBuffer(data)
		}
		return nil, err
	}
	return data, nil
}

func retryRead(cfg *config.Config, src *os.File, offset int64, logger *zap.Logger) ([]byte, error) {
	blockSize := cfg.BlockSize
	maxRetries := cfg.MaxRetries

	var buf []byte
	if cfg.ODirect {
		buf = getAlignedBlockBuffer(blockSize)
	} else {
		buf = getBlockBuffer(blockSize)
	}
	for attempt := 0; attempt < maxRetries; attempt++ {
		n, err := src.ReadAt(buf, offset)
		if err == nil && n == blockSize {
			return buf, nil
		}

		if logger != nil {
			logger.Warn("Failed to read block",
				zap.Int64("offset", offset),
				zap.Int("size_bytes", blockSize),
				zap.Int("attempt", attempt+1),
				zap.Error(err))
		}

		time.Sleep(100 * time.Millisecond)
	}

	if cfg.ODirect {
		putAlignedBlockBuffer(buf)
	} else {
		putBlockBuffer(buf)
	}
	return nil, fmt.Errorf("failed to read block at offset %d after %d attempts", offset, maxRetries)
}
