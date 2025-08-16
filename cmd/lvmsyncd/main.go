package main

import (
	"os"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/exitcode"
)

func run(newLogger func() (*zap.Logger, error), runner *Runner) int {
	logger, err := newLogger()
	if err != nil {
		return exitcode.ErrConfig
	}
	defer rootcmd.SyncLogger(logger)
	if err := runner.Execute(nil, logger); err != nil {
		logger.Error("run_failed", zap.Error(err))
		return exitcode.ErrRuntime
	}
	return exitcode.OK
}

func main() {
	os.Exit(run(func() (*zap.Logger, error) { return zap.NewProduction() }, NewRunner()))
}
