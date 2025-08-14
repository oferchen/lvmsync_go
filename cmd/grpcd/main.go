package main

import (
	"os"

	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		os.Exit(1)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			logger.Error("logger sync error", zap.Error(err))
		}
	}()
	if err := Execute(nil, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		os.Exit(1)
	}
}
