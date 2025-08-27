package transfer

import (
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func findResumeIndexHybrid(cfg *config.Config, ranges []Range, chk resumeChunk, logger *zap.Logger) int {
	return findResumeIndexCDC(cfg, ranges, chk, logger)
}
