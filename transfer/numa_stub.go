//go:build !linux

package transfer

import (
	"os"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

func pinWorkerToDevice(cfg *config.Config, src *os.File, _ *zap.Logger) func() {
	return func() {}
}
