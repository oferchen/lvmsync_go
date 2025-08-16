package transfer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/config"
)

func (t *Transfer) processDumpDataCore(ctx context.Context, cfg *config.Config, in io.Reader, destPath string, dedup DeduplicationStrategy, verify bool) (err error) {
	defer func() { _ = t.Logger.Sync() }()
	bufReader := bufio.NewReader(in)
	var hs common.Handshake
	hs, err = readAndValidateHandshake(cfg, bufReader, dedup, verify)
	if err != nil {
		return err
	}

	decReader, err := NewDecompressionReader(bufReader, hs.Compress, cfg.CompressConcurrency)
	if err != nil {
		return fmt.Errorf("failed to create decompression reader: %w", err)
	}
	defer func() {
		if closeErr := decReader.Close(); closeErr != nil && t.Logger != nil {
			t.Logger.Warn("Failed to close decompression reader", zap.Error(closeErr))
		}
	}()

	reader := bufio.NewReader(decReader)

	if err := t.verifyDestination(ctx, cfg, destPath); err != nil {
		return err
	}

	var destFile *os.File
	if cfg.ODirect {
		tmp, err2 := os.Open(destPath)
		if err2 != nil {
			return fmt.Errorf("failed to open destination device %s: %w", destPath, err2)
		}
		sector, err2 := DetectSectorSize(tmp)
		_ = tmp.Close()
		if err2 == nil && cfg.BlockSize%sector == 0 {
			if f, direct, err2 := openFileODirect(destPath, os.O_RDWR); err2 == nil && direct {
				destFile = f
			}
		}
	}
	if destFile == nil {
		destFile, err = os.OpenFile(destPath, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("failed to open destination device %s: %w", destPath, err)
		}
	}
	defer common.CloseWithErr(destFile, &err, "close destination device")

	startTime := time.Now()
	checksum := GetChecksumStrategy(cfg.ChecksumAlgorithm)
	bw, err := newBlockWriter(cfg, destFile, dedup, verify, checksum, t.Logger)
	var totalBytes int64
	totalBytes, err = bw.write(reader)
	if err != nil {
		return err
	}

	elapsed := time.Since(startTime)
	if t.Logger != nil {
		t.Logger.Info("applied_changes",
			zap.Int64("size_bytes", totalBytes),
			zap.Int64("duration_ms", elapsed.Milliseconds()),
			zap.Float64("mb_per_s", float64(totalBytes)/elapsed.Seconds()/1048576.0))
	}
	return nil
}

// ProcessDumpDataWithDeduplication applies a dump stream to destPath using the given dedup strategy without checksum verification, updating the strategy's state.
func (t *Transfer) ProcessDumpDataWithDeduplication(ctx context.Context, cfg *config.Config, in io.Reader, destPath string, dedup DeduplicationStrategy) error {
	return t.processDumpDataCore(ctx, cfg, in, destPath, dedup, false)
}

// ProcessDumpData applies a dump stream to destPath with checksum verification for each block before writing.
func (t *Transfer) ProcessDumpData(ctx context.Context, cfg *config.Config, in io.Reader, destPath string) error {
	return t.processDumpDataCore(ctx, cfg, in, destPath, nil, true)
}
