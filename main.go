package main

import (
	"os"
	"runtime"

	"go.uber.org/zap"

	_ "lvmsync_go/cmd/apply"
	_ "lvmsync_go/cmd/dump"
	rootcmd "lvmsync_go/cmd/root"
)

var (
	configureFunc     = rootcmd.Configure
	runFunc           = rootcmd.Run
	syncLoggerFunc    = rootcmd.SyncLogger
	exitFunc          = os.Exit
	exampleLoggerFunc = func() *zap.Logger {
		logger, err := zap.NewProduction()
		if err != nil {
			return zap.NewNop()
		}
		return logger
	}
	runtimeGOOS = runtime.GOOS
)

func syncLogger(logger *zap.Logger) { syncLoggerFunc(logger) }

func main() {
	if runtimeGOOS != "linux" {
		tmpLogger := exampleLoggerFunc()
		tmpLogger.Error("unsupported platform", zap.String("goos", runtimeGOOS))
		if err := tmpLogger.Sync(); err != nil {
			tmpLogger.Error("Logger sync error", zap.Error(err))
		}
		exitFunc(1)
		return
	}

	cfg, args, logger, err := configureFunc()
	if err != nil {
		tmpLogger := exampleLoggerFunc()
		tmpLogger.Error("configuration failed", zap.Error(err))
		if err := tmpLogger.Sync(); err != nil {
			tmpLogger.Error("Logger sync error", zap.Error(err))
		}
		exitFunc(1)
		return
	}
	if err := runFunc(cfg, args, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		syncLogger(logger)
		exitFunc(1)
		return
	}
	syncLogger(logger)
}
