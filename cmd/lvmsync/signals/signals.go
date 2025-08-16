package signals

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	"lvmsync_go/lvm"
)

// Handler processes OS signals and performs cleanup.
type Handler interface {
	Handle(ctx context.Context, cfg *config.Config, logger *zap.Logger, signals <-chan os.Signal, snapshotPath *string, errCh chan<- error)
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(ctx context.Context, cfg *config.Config, logger *zap.Logger, signals <-chan os.Signal, snapshotPath *string, errCh chan<- error)

// Handle calls f(ctx, cfg, logger, signals, snapshotPath, errCh).
func (f HandlerFunc) Handle(ctx context.Context, cfg *config.Config, logger *zap.Logger, signals <-chan os.Signal, snapshotPath *string, errCh chan<- error) {
	f(ctx, cfg, logger, signals, snapshotPath, errCh)
}

// Runner holds signal handling dependencies.
type Runner struct {
	RemoveSnapshot func(context.Context, string, *zap.Logger) error
}

// NewRunner constructs a Runner with production dependencies.
func NewRunner() *Runner { return &Runner{RemoveSnapshot: lvm.RemoveSnapshot} }

// NewRunnerWithDeps constructs a Runner with custom dependencies.
func NewRunnerWithDeps(remove func(context.Context, string, *zap.Logger) error) *Runner {
	return &Runner{RemoveSnapshot: remove}
}

// Handle waits for an OS signal, logs it, and removes the snapshot when
// necessary.
func (r *Runner) Handle(ctx context.Context, cfg *config.Config, logger *zap.Logger, signals <-chan os.Signal, snapshotPath *string, errCh chan<- error) {
	sig := <-signals
	logger.Info("received signal, aborting", zap.String("signal", sig.String()))
	if !cfg.SkipSnapshotCreation && *snapshotPath != "" && *snapshotPath != pflag.Arg(0) {
		rmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := r.RemoveSnapshot(rmCtx, *snapshotPath, logger); err != nil {
			logger.Warn("failed to remove snapshot on shutdown", zap.Error(err))
		} else {
			logger.Info("snapshot removed on shutdown", zap.String("snapshot", *snapshotPath))
		}
	}
	errCh <- fmt.Errorf("received signal: %s", sig)
}
