package transfer

import (
	"fmt"
	"os"
	"time"

	"lvmsync_go/config"

	"go.uber.org/zap"
)

func finalizeProgress(cfg *config.Config) {
	if cfg.Progress {
		fmt.Fprintln(os.Stderr, "")
	}
}

func reportProgress(cfg *config.Config, transferred, total int64, index int, start time.Time) {
	if cfg.Progress {
		progressPercent := float64(transferred) / float64(total) * 100.0
		fmt.Fprintf(os.Stderr, "\rProgress: %.2f%%", progressPercent)
	}
	if cfg.Verbose > 0 && index > 0 && index%100 == 0 {
		elapsed := time.Since(start).Seconds()
		speed := float64(transferred) / elapsed / 1048576.0
		Logger.Debug("Parallel dump progress", zap.Int("block", index+1), zap.Float64("MB/s", speed))
	}
}

func logSequentialSummary(bytes int64, skipped int, start time.Time) {
	elapsed := time.Since(start).Seconds()
	Logger.Info("Sequential transfer complete",
		zap.Int64("bytes", bytes),
		zap.Int("skippedBlocks", skipped),
		zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(bytes)/elapsed/1048576.0))
}

func logParallelSummary(bytes int64, start time.Time) {
	elapsed := time.Since(start).Seconds()
	Logger.Info("Parallel transfer complete",
		zap.Int64("bytes", bytes),
		zap.Float64("seconds", elapsed),
		zap.Float64("MB/s", float64(bytes)/elapsed/1048576.0))
}
