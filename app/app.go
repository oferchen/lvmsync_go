package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	signalspkg "lvmsync_go/cmd/lvmsync/signals"
	clientpkg "lvmsync_go/internal/client"
	"lvmsync_go/internal/config"
)

type Runner struct {
	signalsHandler  signalspkg.Handler
	prepareSnapshot func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error)
}

func NewRunner() *Runner {
	return &Runner{
		signalsHandler:  signalspkg.NewRunner(),
		prepareSnapshot: clientpkg.PrepareSnapshot,
	}
}

func NewRunnerWithDeps(deps *Runner) *Runner {
	r := NewRunner()
	if deps == nil {
		return r
	}
	if deps.signalsHandler != nil {
		r.signalsHandler = deps.signalsHandler
	}
	if deps.prepareSnapshot != nil {
		r.prepareSnapshot = deps.prepareSnapshot
	}
	return r
}

func (r *Runner) SetupSignalHandling(ctx context.Context, cfg *config.Config, snapshotPath *string, logger *zap.Logger) (chan os.Signal, chan error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	sigErrCh := make(chan error, 1)
	go r.signalsHandler.Handle(ctx, cfg, logger, signals, snapshotPath, sigErrCh)
	return signals, sigErrCh
}

func (r *Runner) PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return r.prepareSnapshot(ctx, cfg, originalVolume, logger)
}

func SetupSignalHandling(ctx context.Context, cfg *config.Config, snapshotPath *string, logger *zap.Logger) (chan os.Signal, chan error) {
	return NewRunner().SetupSignalHandling(ctx, cfg, snapshotPath, logger)
}

func PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return NewRunner().PrepareSnapshot(ctx, cfg, originalVolume, logger)
}
