package serve

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

// Run returns an error indicating that serve mode is not implemented.
func Run(ctx context.Context, cfg *config.Config, logger *zap.Logger) error {
	logger.Error("serve mode not implemented")
	return fmt.Errorf("serve mode not implemented")
}
