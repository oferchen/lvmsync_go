package main

import (
	"os"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/exitcode"
	"lvmsync_go/internal/logging"
)

func run(newLogger func() (*zap.Logger, error), runner *Runner) int {
	logger, err := newLogger()
	if err != nil {
		return exitcode.ErrConfig
	}
	if err := runner.Execute(nil, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		rootcmd.SyncLogger(logger)
		return exitcode.ErrRuntime
	}
	return exitcode.OK
}

func main() {
	os.Exit(run(func() (*zap.Logger, error) { return logging.NewLogger(nil) }, NewRunner()))
}
