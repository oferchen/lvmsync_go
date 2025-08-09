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

// Handle waits for an OS signal and performs cleanup.
func Handle(cfg *config.Config, signals <-chan os.Signal, snapshotPath *string, errCh chan<- error) {
	sig := <-signals
	zap.L().Info("Received signal, aborting", zap.String("signal", sig.String()))
	if !cfg.SkipSnapshotCreation && *snapshotPath != "" && *snapshotPath != pflag.Arg(0) {
		if err := removeSnapshot(context.Background(), *snapshotPath); err != nil {
			zap.L().Warn("Failed to remove snapshot on shutdown", zap.Error(err))
		} else {
			zap.L().Info("Snapshot removed on shutdown", zap.String("snapshot", *snapshotPath))
		}
	}
	errCh <- fmt.Errorf("received signal: %s", sig)
}
