package main

import (
	"os"

	"go.uber.org/zap"
	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/escalate"
	"lvmsync_go/internal/logging"
)

type runner struct {
	ensureRootOrReexec  func(escalate.Options, *zap.Logger) (bool, error)
	dropToInvokerIfSudo func(escalate.Options, *zap.Logger) error
	syncLogger          func(*zap.Logger)
	exit                func(int)
	newLogger           func() *zap.Logger
}

func newRunner() *runner {
	return &runner{
		ensureRootOrReexec:  escalate.EnsureRootOrReexec,
		dropToInvokerIfSudo: escalate.DropToInvokerIfSudo,
		syncLogger:          rootcmd.SyncLogger,
		exit:                os.Exit,
		newLogger: func() *zap.Logger {
			logger, err := logging.NewLogger(nil, "privdrop")
			if err != nil {
				return zap.NewNop()
			}
			return logger
		},
	}
}

func (r *runner) run(logger *zap.Logger) int {
	reexeced, err := r.ensureRootOrReexec(escalate.Options{}, logger)
	if err != nil {
		logger.Error("ensure_root_or_reexec", zap.Error(err))
		return 1
	}
	if reexeced {
		return 0
	}

	if err := r.dropToInvokerIfSudo(escalate.Options{}, logger); err != nil {
		logger.Error("drop_to_invoker_if_sudo", zap.Error(err))
		return 1
	}

	logger.Info("running_unprivileged")
	return 0
}

func (r *runner) Run() {
	logger := r.newLogger()
	defer r.syncLogger(logger)
	code := r.run(logger)
	r.exit(code)
}

func main() { newRunner().Run() }
