package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
)

var (
	configureFunc  = rootcmd.Configure
	runFunc        = rootcmd.Run
	syncLoggerFunc = rootcmd.SyncLogger
	exitFunc       = os.Exit
)

func syncLogger(logger *zap.Logger) { syncLoggerFunc(logger) }

func main() {
	cfg, logger, err := configureFunc()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration failed: %v\n", err)
		exitFunc(1)
	}
	if err := runFunc(cfg, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		syncLogger(logger)
		exitFunc(1)
	}
	syncLogger(logger)
}
