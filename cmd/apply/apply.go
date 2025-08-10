package apply

import (
	"fmt"

	"lvmsync_go/config"
	"lvmsync_go/transfer"
)

// applyFunc allows tests to override the apply implementation.
var applyFunc = transfer.RunApply

// Run executes apply mode using the provided configuration and arguments.
// args should contain the destination device as the first element.
func Run(cfg *config.Config, applyFile string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("no destination device specified for apply mode")
	}
	destDevice := args[0]
	return applyFunc(cfg, applyFile, destDevice)
}
