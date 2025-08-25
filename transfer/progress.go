package transfer

import (
	"encoding/json"
	"os"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

type progressEvent struct {
	Event            string  `json:"event"`
	Progress         float64 `json:"progress_percent"`
	BytesTransferred int64   `json:"bytes_transferred,omitempty"`
	BytesTotal       int64   `json:"bytes_total,omitempty"`
}

func finalizeProgress(cfg *config.Config, logger *zap.Logger) {
	if cfg.Progress {
		logger.Info("progress complete")
		if cfg.Output == "json" {
			if err := json.NewEncoder(os.Stdout).Encode(progressEvent{Event: "complete", Progress: 100}); err != nil {
				logger.Error("encode progress event", zap.Error(err))
			}
		}
	}
}

func reportProgress(cfg *config.Config, transferred, total int64, index int, start time.Time, logger *zap.Logger) {
	if cfg.Progress {
		progressPercent := float64(transferred) / float64(total) * 100.0

		logger.Info("transfer progress", zap.Float64("progress_percent", progressPercent))
		if cfg.Output == "json" {
			if err := json.NewEncoder(os.Stdout).Encode(progressEvent{
				Event:            "progress",
				Progress:         progressPercent,
				BytesTransferred: transferred,
				BytesTotal:       total,
			}); err != nil {
				logger.Error("encode progress event", zap.Error(err))
			}
		}
	}
	if cfg.Verbose > 0 && index > 0 && index%100 == 0 {
		elapsed := time.Since(start).Seconds()
		speed := float64(transferred) / elapsed / 1048576.0
		logger.Debug("parallel dump progress",
			zap.Int("block_index", index+1),
			zap.Float64("mb_per_sec", speed))
	}
}

func logSequentialSummary(logger *zap.Logger, bytes int64, skipped int, start time.Time) {
	elapsed := time.Since(start).Seconds()
	logger.Info("Sequential transfer complete",
		zap.Int64("size_bytes", bytes),
		zap.Int("skipped_blocks", skipped),
		zap.Float64("duration_sec", elapsed),
		zap.Float64("mb_per_sec", float64(bytes)/elapsed/1048576.0))
}

func logParallelSummary(logger *zap.Logger, bytes int64, start time.Time) {
	elapsed := time.Since(start).Seconds()
	logger.Info("Parallel transfer complete",
		zap.Int64("size_bytes", bytes),
		zap.Float64("duration_sec", elapsed),
		zap.Float64("mb_per_sec", float64(bytes)/elapsed/1048576.0))
}
