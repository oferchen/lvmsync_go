package main

import (
	"os"

	"go.uber.org/zap"
	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/escalate"
)

func main() {
	logger := zap.NewExample()
	defer rootcmd.SyncLogger(logger)

	reexeced, err := escalate.EnsureRootOrReexec(escalate.Options{}, logger)
	if err != nil {
		logger.Error("ensure_root_or_reexec", zap.Error(err))
		os.Exit(1)
	}
	if reexeced {
		return
	}

	// privileged work would run here

	if err := escalate.DropToInvokerIfSudo(escalate.Options{}, logger); err != nil {
		logger.Error("drop_to_invoker_if_sudo", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("running_unprivileged")
}
