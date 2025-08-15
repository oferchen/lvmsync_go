package transfer

import (
	"crypto/sha256"
	"os"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/config"
)

// findResumeIndex determines the starting range index based on the checkpoint and dedup mode.
func findResumeIndex(cfg *config.Config, srcFile *os.File, ranges []Range, chk resumeCheckpoint, logger *zap.Logger) int {
	if cfg.ResumeState == "" {
		return 0
	}
	switch cfg.DedupMode {
	case "cdc":
		return findResumeIndexCDC(cfg, ranges, chk, logger)
	case "hybrid":
		return findResumeIndexHybrid(cfg, ranges, chk, logger)
	default:
		return findResumeIndexFixed(cfg, srcFile, ranges, chk, logger)
	}
}

func findResumeIndexFixed(cfg *config.Config, srcFile *os.File, ranges []Range, chk resumeCheckpoint, logger *zap.Logger) int {
	if chk.Chunk == [32]byte{} {
		return 0
	}
	for i, r := range ranges {
		offset, _, err := validateOffsetAndSize(r.Start, cfg.BlockSize)
		if err != nil {
			return 0
		}
		data, err := ReadBlockWithRetries(cfg, srcFile, offset, cfg.ZeroCopy, [2]int{-1, -1}, logger)
		if err != nil {
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
			if logger != nil {
				logger.Info("Resuming after index", zap.Int("resume_index", i+1))
			}
			return i + 1
		}
	}
	return 0
}
