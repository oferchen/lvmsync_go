package serve

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

// Run returns an error indicating that serve mode is not implemented. It wraps
// server start in a cancellable context and ensures logs are flushed on
// shutdown.
func Run(parent context.Context, cfg *config.Config, logger *zap.Logger) (context.Context, error) {
	ctx, cancel := context.WithCancel(parent)
	defer func() {
		cancel()
		if err := logger.Sync(); err != nil {
			logger.Error("sync_error", zap.Error(err))
		}
	}()

	logger.Error("serve mode not implemented")
	return ctx, fmt.Errorf("serve mode not implemented")
}
