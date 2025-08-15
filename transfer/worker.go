package transfer

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/config"
	"lvmsync_go/internal/blocksize"
)

// detectBlockSize sets cfg.BlockSize by probing source; logger must be non-nil.
func detectBlockSize(cfg *config.Config, source string, logger *zap.Logger) error {
	if cfg.BlockSize != 0 {
		return nil
	}
	bs, err := blocksize.Detect(source)
	if err != nil {
		return fmt.Errorf("auto-detect block size: %w", err)
	}
	cfg.BlockSize = bs
	logger.Info("Auto-detected block size", zap.Int("block_size_bytes", cfg.BlockSize))
	return nil
}

// gatherChangedRanges returns ranges of changed blocks; logger must be non-nil.
func gatherChangedRanges(snapshot string, blockSize int64, logger *zap.Logger) ([]Range, error) {
	metadataDevice := GetMetadataDevice(snapshot)
	if metadataDevice == "" {
		return nil, fmt.Errorf("failed to determine metadata device from snapshot %s", snapshot)
	}
	ranges, err := GetDifferences(metadataDevice, blockSize)
	if err != nil {
		return nil, fmt.Errorf("error getting differences: %w", err)
	}
	logger.Info("Changed blocks determined", zap.Int("block_count", len(ranges)))
	return ranges, nil
}

// prepareRanges calculates changed block ranges; logger must be non-nil.
func prepareRanges(cfg *config.Config, snapshot, source string, logger *zap.Logger) ([]Range, error) {
	if err := detectBlockSize(cfg, source, logger); err != nil {
		return nil, err
	}
	logger.Info("Using block size", zap.Int("block_size_bytes", cfg.BlockSize))

	_, blockSize, err := validateOffsetAndSize(0, cfg.BlockSize)
	if err != nil {
		return nil, err
	}
	ranges, err := gatherChangedRanges(snapshot, int64(blockSize), logger)
	if err != nil {
		return nil, err
	}
	return ranges, nil
}

func setupSourceFile(cfg *config.Config, source string) (*os.File, error) {
	if cfg.ODirect {
		tmp, err := os.Open(source)
		if err != nil {
			return nil, fmt.Errorf("failed to open source device %s: %w", source, err)
		}
		sector, err := DetectSectorSize(tmp)
		_ = tmp.Close()
		if err == nil && cfg.BlockSize%sector == 0 {
			if f, direct, err := openFileODirect(source, os.O_RDONLY); err == nil && direct {
				return f, nil
			}
		}
	}
	srcFile, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("failed to open source device %s: %w", source, err)
	}
	return srcFile, nil
}

// setupPipe initializes optional zero-copy pipe; logger must be non-nil.
func setupPipe(cfg *config.Config, logger *zap.Logger) ([2]int, func(), error) {
	var pipeFds [2]int
	cleanup := func() {}
	if cfg.ZeroCopy {
		if err := syscall.Pipe(pipeFds[:]); err != nil {
			return pipeFds, cleanup, fmt.Errorf("failed to create pipe: %w", err)
		}
		cleanup = func() {
			if closeErr := syscall.Close(pipeFds[0]); closeErr != nil {
				logger.Warn("close pipe", zap.Int("fd", pipeFds[0]), zap.Error(closeErr))
			}
			if closeErr := syscall.Close(pipeFds[1]); closeErr != nil {
				logger.Warn("close pipe", zap.Int("fd", pipeFds[1]), zap.Error(closeErr))
			}
		}
	} else {
		pipeFds[0], pipeFds[1] = -1, -1
	}
	return pipeFds, cleanup, nil
}

// startParallelWorkers launches worker goroutines; logger must be non-nil.
func (t *Transfer) startParallelWorkers(cfg *config.Config, srcFile *os.File, ranges []Range, resumeStart int, logger *zap.Logger) <-chan *BlockResult {
	numBlocks := len(ranges)
	taskBuf := cfg.Parallel
	if taskBuf < 1 {
		taskBuf = 1
	}
	tasks := make(chan BlockTask, taskBuf)
	results := make(chan *BlockResult, taskBuf)
	for i := 0; i < cfg.Parallel; i++ {
		t.workerWG.Add(1)
		go worker(cfg, srcFile, tasks, results, t.workerWG, logger)
	}

	go func() {
		for i := resumeStart; i < numBlocks; i++ {
			tasks <- BlockTask{Index: i, R: ranges[i]}
		}
		close(tasks)
	}()

	go finalizeResults(t.workerWG, results)
	return results
}

// DumpChangesParallel streams changes in parallel; logger must be non-nil.
func (t *Transfer) DumpChangesParallel(cfg *config.Config, snapshot, source string, out io.Writer) (err error) {
	if cfg.ZeroCopy {
		t.Logger.Warn("ZeroCopy mode enabled, falling back to sequential execution")
		return t.DumpChangesSequential(cfg, snapshot, source, out)
	}

	ranges, err := prepareRanges(cfg, snapshot, source, t.Logger)
	if err != nil {
		return err
	}

	totalDataSize := calculateTotalDataSize(ranges)
	if err = writeParallelHandshake(cfg, out); err != nil {
		return err
	}

	compWriter, bufOut, err := prepareOutputWriter(out, cfg, t.Logger)
	if err != nil {
		return err
	}
	defer cleanupOutput(bufOut, compWriter, t.Logger)

	srcFile, err := setupSourceFile(cfg, source)
	if err != nil {
		return err
	}
	defer common.CloseWithErr(srcFile, &err, "close source file")

	checkpoint := readResumeState(cfg, t.Logger)
	resumeStart := findResumeIndex(cfg, srcFile, ranges, checkpoint, t.Logger)
	results := t.startParallelWorkers(cfg, srcFile, ranges, resumeStart, t.Logger)

	startTime := time.Now()
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	var totalBytesTransferred int64
	var finalDigest []byte
	totalBytesTransferred, finalDigest, err = processParallelResults(cfg, results, bufOut, checksum, totalDataSize, startTime, t.Logger, t.Tracker)
	if err != nil {
		return err
	}
	finalizeProgress(cfg, t.Logger)
	logParallelSummary(t.Logger, totalBytesTransferred, startTime)
	finalizeResumeState(cfg, t.Tracker, t.Logger)
	if len(finalDigest) > 0 {
		t.Logger.Info("final checksum", zap.String("final_digest", fmt.Sprintf("%x", finalDigest)))
	}
	_ = t.Logger.Sync()
	return nil
}
