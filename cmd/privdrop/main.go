package main

import (
	"os"

	"go.uber.org/zap"
	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/escalate"
)

type runner struct {
	ensureRootOrReexec  func(escalate.Options, *zap.Logger) (bool, error)
	dropToInvokerIfSudo func(escalate.Options, *zap.Logger) error
}

func newRunner() *runner {
	return &runner{
		ensureRootOrReexec:  escalate.EnsureRootOrReexec,
		dropToInvokerIfSudo: escalate.DropToInvokerIfSudo,
	}
}

func newRunnerWithDeps(ensure func(escalate.Options, *zap.Logger) (bool, error), drop func(escalate.Options, *zap.Logger) error) *runner {
	return &runner{ensureRootOrReexec: ensure, dropToInvokerIfSudo: drop}
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

func main() {
	logger := zap.NewExample()
	defer rootcmd.SyncLogger(logger)

	r := newRunner()
	os.Exit(r.run(logger))
}
