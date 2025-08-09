package transfer

import (
	"time"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

func finalizeProgress(cfg *config.Config) {
	if cfg.Progress && Logger != nil {
		Logger.Info("progress complete")
	}
}

func reportProgress(cfg *config.Config, transferred, total int64, index int, start time.Time) {
	if Logger == nil {
		return
	}
	if cfg.Progress {
		progressPercent := float64(transferred) / float64(total) * 100.0

		Logger.Info("transfer progress", zap.Float64("progress_percent", progressPercent))
	}
	if cfg.Verbose > 0 && index > 0 && index%100 == 0 {
		elapsed := time.Since(start).Seconds()
		speed := float64(transferred) / elapsed / 1048576.0
		Logger.Debug("parallel dump progress",
			zap.Int("block_index", index+1),
			zap.Float64("mb_per_sec", speed))
	}
}

func logSequentialSummary(bytes int64, skipped int, start time.Time) {
	if Logger == nil {
		return
	}
	elapsed := time.Since(start).Seconds()
	Logger.Info("Sequential transfer complete",
		zap.Int64("size_bytes", bytes),
		zap.Int("skipped_blocks", skipped),
		zap.Float64("duration_sec", elapsed),
		zap.Float64("mb_per_sec", float64(bytes)/elapsed/1048576.0))
}

func logParallelSummary(bytes int64, start time.Time) {
	if Logger == nil {
		return
	}
	elapsed := time.Since(start).Seconds()
	Logger.Info("Parallel transfer complete",
		zap.Int64("size_bytes", bytes),
		zap.Float64("duration_sec", elapsed),
		zap.Float64("mb_per_sec", float64(bytes)/elapsed/1048576.0))
}
