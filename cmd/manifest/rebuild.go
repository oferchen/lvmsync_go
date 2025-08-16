package manifest

import (
	"context"
	"fmt"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/config"
	manifestpkg "lvmsync_go/manifest"
)

var rebuildFn = manifestpkg.Rebuild

func init() {
	rootcmd.RunManifest = Run
}

// Run executes manifest subcommands. Currently only "rebuild" is supported.
func Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	defer rootcmd.SyncLogger(logger)
	if len(args) == 0 || args[0] != "rebuild" {
		fs := pflag.NewFlagSet("manifest", pflag.ContinueOnError)
		fs.Usage()
		return fmt.Errorf("usage: lvmsync manifest rebuild <device>")
	}
	flagSets := config.NewFlagSets(cfg)
	flagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	flagSets.Remote = pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
	flagSets.Compression = pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	flagSets.LVM = pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
	flagSets.GRPC = pflag.NewFlagSet("gRPC Options", pflag.ExitOnError)
	flagSets.Transport = pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
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
	logger.Info("rebuilding manifest", zap.String("device", device), zap.String("output", path))
	if conf.DryRun {
		logger.Info("dry run - skipping rebuild")
		return nil
	}
	ctx := context.Background()
	var cancel context.CancelFunc
	if conf.ManifestTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, conf.ManifestTimeout)
		defer cancel()
	}
	hybridFixed := uint32(0)
	if conf.DedupMode == "hybrid" {
		hybridFixed = uint32(conf.BlockSize)
	}
	if err := rebuildFn(ctx, device, path, logger, conf.ManifestProgressInterval, conf.ManifestAllowMounted, uint32(conf.CDCMin), uint32(conf.CDCAvg), uint32(conf.CDCMax), hybridFixed); err != nil {
		logger.Error("rebuild failed", zap.Error(err))
		return err
	}
	logger.Info("rebuild complete")
	return nil
}
