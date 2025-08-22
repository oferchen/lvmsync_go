// Package main contains the deprecated serve command.
package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	rootcmd "github.com/oferchen/lvmsync_go/cmd/root"
	"github.com/oferchen/lvmsync_go/internal/logging"
	_ "github.com/oferchen/lvmsync_go/transport/rsyncwire"
)

const deprecationMsg = "serve command deprecated; use lvmsyncd instead"

type runner struct {
	run        func([]string, *zap.Logger) int
	syncLogger func(*zap.Logger)
	exit       func(int)
	newLogger  func() *zap.Logger
}

func newRunner() *runner {
	return &runner{
		run:        run,
		syncLogger: rootcmd.SyncLogger,
		exit:       os.Exit,
		newLogger: func() *zap.Logger {
			// Deprecated command, but keep logger consistent across commands.
			logger, err := logging.NewLogger(nil, "serve")
			if err != nil {
				return zap.NewNop()
			}
			return logger
		},
	}
}

func newRunnerWithDeps(runFunc func([]string, *zap.Logger) int, syncFunc func(*zap.Logger), exitFunc func(int), loggerFunc func() *zap.Logger) *runner {
	r := newRunner()
	if runFunc != nil {
		r.run = runFunc
	}
	if syncFunc != nil {
		r.syncLogger = syncFunc
	}
	if exitFunc != nil {
		r.exit = exitFunc
	}
	if loggerFunc != nil {
		r.newLogger = loggerFunc
	}
	return r
}

func (r *runner) Run(args []string) {
	logger := r.newLogger()
	defer r.syncLogger(logger)
	code := r.run(args, logger)
	r.exit(code)
}

func newCommand(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Deprecated server command",
		RunE: func(_ *cobra.Command, _ []string) error {
			logger.Error("command_deprecated", zap.String("replacement", "lvmsyncd"))
			return errors.New(deprecationMsg)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetHelpFunc(func(_ *cobra.Command, _ []string) {
		logger.Warn(deprecationMsg)
	})
	return cmd
}

func run(args []string, logger *zap.Logger) int {
	cmd := newCommand(logger)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return 1
	}
	return 0
}

func main() { newRunner().Run(os.Args[1:]) }
