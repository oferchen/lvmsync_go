//go:build !linux

package transfer

import (
	"os"

	"lvmsync_go/config"
)

func pinWorkerToDevice(cfg *config.Config, src *os.File) func() {
	return func() {}
}
