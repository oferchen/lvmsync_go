package manifest

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/config"
	manifestpkg "lvmsync_go/manifest"
)

// Runner holds dependencies for manifest operations.
type Runner struct {
	Rebuild func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error
}

// NewRunner returns a Runner with production dependencies.
func NewRunner() *Runner { return &Runner{Rebuild: manifestpkg.Rebuild} }

// NewRunnerWithDeps creates a Runner with custom rebuild function.
func NewRunnerWithDeps(
	rebuild func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error,
) *Runner {
	return &Runner{Rebuild: rebuild}
}

func init() {
	r := NewRunner()
	rootcmd.RegisterManifest(r.Run)
}

// Run executes manifest subcommands. Currently only "rebuild" is supported.
func (r *Runner) Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	defer rootcmd.SyncLogger(logger)
	if len(args) == 0 || args[0] != "rebuild" {
		fs := pflag.NewFlagSet("manifest", pflag.ContinueOnError)
		fs.Usage()
		return fmt.Errorf("usage: lvmsync manifest rebuild <device>")
	}
	builder := config.NewBuilder(cfg)
	builder.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	builder.FlagSets.Remote = pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
	builder.FlagSets.Compression = pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	builder.FlagSets.LVM = pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
	builder.FlagSets.Transport = pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
	fs := pflag.NewFlagSet("manifest rebuild", pflag.ContinueOnError)
	conf, remaining, warns, err := builder.Build(fs, args[1:])
	if err != nil {
		return err
	}
	for _, w := range warns {
		logger.Warn("config_warning", zap.String("detail", w))
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
	if err := r.Rebuild(ctx, device, path, logger, conf.ManifestProgressInterval, conf.ManifestAllowMounted, uint32(conf.CDCMin), uint32(conf.CDCAvg), uint32(conf.CDCMax), hybridFixed); err != nil {
		logger.Error("rebuild failed", zap.Error(err))
		return err
	}
	logger.Info("rebuild complete")
	return nil
}

// Run executes using a default Runner.
func Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	return NewRunner().Run(cfg, args, logger)
}
