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
	Regenerate func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error
	GC         func(path string, opts ...manifestpkg.IndexOption) error
	Compact    func(path string, opts ...manifestpkg.IndexOption) error
	Open       func(path string, opts ...manifestpkg.IndexOption) (*manifestpkg.Index, error)
}

// NewRunner returns a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{Regenerate: manifestpkg.Regenerate, GC: manifestpkg.GC, Compact: manifestpkg.Compact, Open: manifestpkg.Open}
}

// NewRunnerWithDeps creates a Runner with custom regenerate function.
func NewRunnerWithDeps(
	regenerate func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error,
	gc func(path string, opts ...manifestpkg.IndexOption) error,
	compact func(path string, opts ...manifestpkg.IndexOption) error,
	open func(path string, opts ...manifestpkg.IndexOption) (*manifestpkg.Index, error),
) *Runner {
	return &Runner{Regenerate: regenerate, GC: gc, Compact: compact, Open: open}
}

func init() {
	r := NewRunner()
	rootcmd.RegisterManifest(r.Run)
}

// Run executes manifest subcommands.
func (r *Runner) Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	defer rootcmd.SyncLogger(logger)
	if len(args) == 0 {
		fs := pflag.NewFlagSet("manifest", pflag.ContinueOnError)
		fs.Usage()
		return fmt.Errorf("usage: lvmsync manifest <rebuild|gc|compact|check>")
	}
	switch args[0] {
	case "rebuild":
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
		if err := r.Regenerate(ctx, device, path, logger, conf.ManifestProgressInterval, conf.ManifestAllowMounted, uint32(conf.CDCMin), uint32(conf.CDCAvg), uint32(conf.CDCMax), hybridFixed); err != nil {
			logger.Error("rebuild failed", zap.Error(err))
			return err
		}
		logger.Info("rebuild complete")
		return nil
	case "gc":
		if len(args) != 2 {
			fs := pflag.NewFlagSet("manifest gc", pflag.ContinueOnError)
			fs.Usage()
			return fmt.Errorf("usage: lvmsync manifest gc <path>")
		}
		path := args[1]
		logger.Info("manifest_gc", zap.String("path", path))
		if cfg.DryRun {
			logger.Info("dry run - skipping gc")
			return nil
		}
		if err := r.GC(path); err != nil {
			logger.Error("gc failed", zap.Error(err))
			return err
		}
		logger.Info("gc complete")
		return nil
	case "compact":
		if len(args) != 2 {
			fs := pflag.NewFlagSet("manifest compact", pflag.ContinueOnError)
			fs.Usage()
			return fmt.Errorf("usage: lvmsync manifest compact <path>")
		}
		path := args[1]
		logger.Info("manifest_compact", zap.String("path", path))
		if cfg.DryRun {
			logger.Info("dry run - skipping compact")
			return nil
		}
		if err := r.Compact(path); err != nil {
			logger.Error("compact failed", zap.Error(err))
			return err
		}
		logger.Info("compact complete")
		return nil
	case "check":
		if len(args) != 2 {
			fs := pflag.NewFlagSet("manifest check", pflag.ContinueOnError)
			fs.Usage()
			return fmt.Errorf("usage: lvmsync manifest check <path>")
		}
		path := args[1]
		logger.Info("manifest_check", zap.String("path", path))
		if cfg.DryRun {
			logger.Info("dry run - skipping check")
			return nil
		}
		idx, err := r.Open(path)
		if err != nil {
			logger.Error("manifest_corrupt", zap.Error(err))
			return err
		}
		for i := uint64(0); i < idx.ChunkCount(); i++ {
			if _, _, _, _, _, err := idx.Entry(i); err != nil {
				idx.Close()
				logger.Error("manifest_corrupt", zap.Error(err))
				return err
			}
		}
		if err := idx.Close(); err != nil {
			logger.Error("manifest_check_close_error", zap.Error(err))
			return err
		}
		logger.Info("manifest_ok")
		return nil
	default:
		fs := pflag.NewFlagSet("manifest", pflag.ContinueOnError)
		fs.Usage()
		return fmt.Errorf("usage: lvmsync manifest <rebuild|gc|compact|check>")
	}
}

// Run executes using a default Runner.
func Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	return NewRunner().Run(cfg, args, logger)
}
