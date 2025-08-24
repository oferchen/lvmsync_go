package main

import (
	"os"
	"runtime"

	"go.uber.org/zap"

	_ "lvmsync_go/cmd/dump"
	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/config"
	"lvmsync_go/internal/exitcode"
	"lvmsync_go/internal/logging"
)

// Runner executes the application with injected dependencies.
type Runner struct {
	Configure     func() (*config.Config, []string, *zap.Logger, error)
	RunFunc       func(*config.Config, []string, *zap.Logger) error
	SyncLogger    func(*zap.Logger)
	Exit          func(int)
	ExampleLogger func() *zap.Logger
	GOOS          string
}

// NewRunner constructs a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{
		Configure:  rootcmd.Configure,
		RunFunc:    rootcmd.Run,
		SyncLogger: rootcmd.SyncLogger,
		Exit:       os.Exit,
		ExampleLogger: func() *zap.Logger {
			logger, err := logging.NewLogger(nil, "main")
			if err != nil {
				return zap.NewNop()
			}
			return logger
		},
		GOOS: runtime.GOOS,
	}
}

// NewRunnerWithDeps constructs a Runner with custom dependencies, useful for tests.
func NewRunnerWithDeps(
	configure func() (*config.Config, []string, *zap.Logger, error),
	run func(*config.Config, []string, *zap.Logger) error,
	syncLogger func(*zap.Logger),
	exit func(int),
	exampleLogger func() *zap.Logger,
	goos string,
) *Runner {
	return &Runner{
		Configure:     configure,
		RunFunc:       run,
		SyncLogger:    syncLogger,
		Exit:          exit,
		ExampleLogger: exampleLogger,
		GOOS:          goos,
	}
}

// Run executes the configured application flow.
func (r *Runner) Run() {
	if r.GOOS != "linux" {
		tmpLogger := r.ExampleLogger()
		tmpLogger.Error("unsupported platform", zap.String("goos", r.GOOS))
		rootcmd.SyncLogger(tmpLogger)
		r.Exit(1)
		return
	}

	cfg, args, logger, err := r.Configure()
	if err != nil {
		tmpLogger := r.ExampleLogger()
		tmpLogger.Error("configuration failed", zap.Error(err))
		rootcmd.SyncLogger(tmpLogger)
		r.Exit(rootcmd.ExitCode(err))
		return
	}
	if err := r.RunFunc(cfg, args, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		r.SyncLogger(logger)
		r.Exit(rootcmd.ExitCode(err))
		return
	}
	r.SyncLogger(logger)
	r.Exit(exitcode.OK)
}

func main() {
	NewRunner().Run()
}
