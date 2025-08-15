package transfer

import (
	"go.uber.org/zap"

	"lvmsync_go/config"
)

func findResumeIndexHybrid(cfg *config.Config, ranges []Range, chk resumeCheckpoint, logger *zap.Logger) int {
	return findResumeIndexCDC(cfg, ranges, chk, logger)
}
