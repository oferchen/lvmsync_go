package transfer

import (
	"go.uber.org/zap"

	"lvmsync_go/config"
)

// findResumeIndexCDC resumes scanning using CDC; logger must be non-nil.
func findResumeIndexCDC(cfg *config.Config, ranges []Range, chk resumeChunk, logger *zap.Logger) int {
	next := chk.Offset + uint64(chk.Length)
	for i := range ranges {
		if next < ranges[i].Start {
			return i
		}
		if next <= ranges[i].End {
			ranges[i].Start = next
			logger.Info("Resuming after offset", zap.Uint64("resume_offset", next))
			return i
		}
	}
	return len(ranges)
}
