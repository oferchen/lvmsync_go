package main

import (
	"os"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
)

var (
	configureFunc     = rootcmd.Configure
	runFunc           = rootcmd.Run
	syncLoggerFunc    = rootcmd.SyncLogger
	exitFunc          = os.Exit
	exampleLoggerFunc = func() *zap.Logger { return zap.NewExample() }
)

func syncLogger(logger *zap.Logger) { syncLoggerFunc(logger) }

func main() {
	cfg, logger, err := configureFunc()
	if err != nil {
		tmpLogger := exampleLoggerFunc()
		tmpLogger.Error("configuration failed", zap.Error(err))
		_ = tmpLogger.Sync()
		exitFunc(1)
		return
	}
	if err := runFunc(cfg, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		syncLogger(logger)
		exitFunc(1)
		return
	}
	syncLogger(logger)
}
