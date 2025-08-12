package signals

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/lvm"
)

var removeSnapshot = lvm.RemoveSnapshot

// Handle waits for an OS signal, logs it, and removes the snapshot when
// necessary.
func Handle(cfg *config.Config, logger *zap.Logger, signals <-chan os.Signal, snapshotPath *string, errCh chan<- error) {
	sig := <-signals
	logger.Info("received signal, aborting", zap.String("signal", sig.String()))
	if !cfg.SkipSnapshotCreation && *snapshotPath != "" && *snapshotPath != pflag.Arg(0) {
		if err := removeSnapshot(context.Background(), *snapshotPath); err != nil {
			logger.Warn("failed to remove snapshot on shutdown", zap.Error(err))
		} else {
			logger.Info("snapshot removed on shutdown", zap.String("snapshot", *snapshotPath))
		}
	}
	errCh <- fmt.Errorf("received signal: %s", sig)
}
