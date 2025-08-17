package transfer

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

// findResumeIndex determines the starting range index based on the checkpoint and dedup mode.
func findResumeIndex(ctx context.Context, cfg *config.Config, srcFile *os.File, ranges []Range, chk resumeCheckpoint, logger *zap.Logger) int {
	if cfg.ResumeState == "" {
		return 0
	}
	switch cfg.DedupMode {
	case "cdc":
		return findResumeIndexCDC(cfg, ranges, chk.CDC, logger)
	case "hybrid":
		return findResumeIndexHybrid(cfg, ranges, chk.Hybrid, logger)
	default:
		return findResumeIndexFixed(ctx, cfg, srcFile, ranges, chk.Fixed, logger)
	}
}

// findResumeIndexFixed finds resume index using fixed-size blocks; logger must be non-nil.
func findResumeIndexFixed(ctx context.Context, cfg *config.Config, srcFile *os.File, ranges []Range, chk resumeChunk, logger *zap.Logger) int {
	if chk.Chunk == [32]byte{} {
		return 0
	}
	for i, r := range ranges {
		offset, _, err := validateOffsetAndSize(r.Start, cfg.BlockSize)
		if err != nil {
			return 0
		}
		data, err := ReadBlockWithRetries(ctx, cfg, srcFile, offset, cfg.ZeroCopy, [2]int{-1, -1}, logger)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return 0
			}
			continue
		}
		var sum [32]byte
		if cfg.ChecksumAlgorithm == "sha256" {
			sumArr := sha256.Sum256(data)
			sum = [32]byte(sumArr)
		} else {
			sum = blake3.Sum256(data)
		}
		putBlockBuffer(data)
		if sum == chk.Chunk {
			logger.Info("Resuming after index", zap.Int("resume_index", i+1))
			return i + 1
		}
	}
	return 0
}
