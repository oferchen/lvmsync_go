package manifest

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"lvmsync_go/config"
	manifestpkg "lvmsync_go/manifest"
)

// Run executes manifest subcommands. Currently only "rebuild" is supported.
func Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	if logger != nil {
		defer logger.Sync()
	}
	if len(args) == 0 || args[0] != "rebuild" {
		fs := pflag.NewFlagSet("manifest", pflag.ContinueOnError)
		fs.Usage()
		return fmt.Errorf("usage: lvmsync manifest rebuild <device>")
	}
	flagSets := config.NewFlagSets(cfg)
	fs := pflag.NewFlagSet("manifest rebuild", pflag.ContinueOnError)
	conf, remaining, err := config.LoadConfig(flagSets, cfg, fs, args[1:])
	if err != nil {
		return err
	}
	if len(remaining) != 1 {
		fs.Usage()
		return fmt.Errorf("usage: lvmsync manifest rebuild <device>")
	}
	device := remaining[0]
	path := conf.ManifestPath
	if path == "" {
		path = device + ".manifest"
	}
	if logger != nil {
		logger.Info("rebuilding manifest", zap.String("device", device), zap.String("output", path))
	}
	if conf.DryRun {
		if logger != nil {
			logger.Info("dry run - skipping rebuild")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := manifestpkg.Rebuild(ctx, device, path, logger, conf.ManifestProgressInterval, conf.ManifestAllowMounted); err != nil {
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
