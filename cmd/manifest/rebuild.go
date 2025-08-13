package manifest

import (
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"lvmsync_go/config"
	manifestpkg "lvmsync_go/manifest"
)

// Run executes manifest subcommands. Currently only "rebuild" is supported.
func Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	fs := pflag.NewFlagSet("manifest", pflag.ContinueOnError)
	output := fs.String("output", "", "manifest output file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	v := viper.New()
	_ = v.BindPFlags(fs)
	remaining := fs.Args()
	if len(remaining) < 2 || remaining[0] != "rebuild" {
		fs.Usage()
		return fmt.Errorf("usage: lvmsync manifest rebuild <device>")
	}
	device := remaining[1]
	path := *output
	if path == "" {
		path = device + ".manifest"
	}
	if logger != nil {
		logger.Info("rebuilding manifest", zap.String("device", device), zap.String("output", path))
	}
	if err := manifestpkg.Rebuild(device, path); err != nil {
		if logger != nil {
			logger.Error("rebuild failed", zap.Error(err))
		}
		return err
	}
	if logger != nil {
		logger.Info("rebuild complete")
	}
	return nil
}
