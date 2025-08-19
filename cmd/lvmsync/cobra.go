package lvmsync

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	verifycmd "lvmsync_go/cmd/verify"
	"lvmsync_go/internal/config"
	"lvmsync_go/manifest"
	"lvmsync_go/transfer"
)

// RunOptions collects flags for the run command.
type RunOptions struct {
	DryRun    bool
	Transport string
	Delta     string
	DedupMode string
	BlockSize int
	CDCMin    int
	CDCAvg    int
	CDCMax    int
	ChunkSeed uint64
	Output    string
}

// Runner holds command behaviors to allow dependency injection in tests.
type Runner struct {
	run             func(src, dst string, opts RunOptions, logger *zap.Logger) error
	manifestRebuild func(device string, cfg *config.Config, logger *zap.Logger) error
	verify          func(args []string, logger *zap.Logger) error
}

// Run executes the main synchronization command.
func (r *Runner) Run(src, dst string, opts RunOptions, logger *zap.Logger) error {
	return r.run(src, dst, opts, logger)
}

// ManifestRebuild regenerates a manifest for the specified device.
func (r *Runner) ManifestRebuild(device string, cfg *config.Config, logger *zap.Logger) error {
	return r.manifestRebuild(device, cfg, logger)
}

// Verify compares source and destination devices against a manifest.
func (r *Runner) Verify(args []string, logger *zap.Logger) error {
	return r.verify(args, logger)
}

// NewRunner constructs a Runner with default no-op behaviors.
func NewRunner() *Runner {
	return &Runner{
		run: func(src, dst string, opts RunOptions, logger *zap.Logger) error { return nil },
		manifestRebuild: func(device string, cfg *config.Config, logger *zap.Logger) error {
			if cfg.DryRun {
				logger.Info("dry run - skipping rebuild")
				return nil
			}
			path := cfg.ManifestPath
			if path == "" {
				path = device + ".manifest"
			}
			ctx := context.Background()
			if cfg.ManifestTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, cfg.ManifestTimeout)
				defer cancel()
			}
			hybridFixed := uint32(0)
			if cfg.DedupMode == "hybrid" {
				hybridFixed = uint32(cfg.BlockSize)
			}
			return manifest.Regenerate(
				ctx,
				device,
				path,
				logger,
				cfg.ManifestProgressInterval,
				cfg.ManifestAllowMounted,
				uint32(cfg.CDCMin),
				uint32(cfg.CDCAvg),
				uint32(cfg.CDCMax),
				hybridFixed,
			)
		},
		verify: func(args []string, logger *zap.Logger) error { return verifycmd.Run(args, logger) },
	}
}

// NewRunnerWithDeps constructs a Runner with custom behaviors, useful for tests.
func NewRunnerWithDeps(
	run func(src, dst string, opts RunOptions, logger *zap.Logger) error,
	rebuild func(device string, cfg *config.Config, logger *zap.Logger) error,
	verify func(args []string, logger *zap.Logger) error,
) *Runner {
	r := NewRunner()
	if run != nil {
		r.run = run
	}
	if rebuild != nil {
		r.manifestRebuild = rebuild
	}
	if verify != nil {
		r.verify = verify
	}
	return r
}

// NewRootCmd creates the root cobra command with all subcommands wired.
func NewRootCmd(logger *zap.Logger, r *Runner) *cobra.Command {
	if r == nil {
		r = NewRunner()
	}
	rootCmd := &cobra.Command{
		Use:   "lvmsync",
		Short: "LVMSync command line tool",
	}

	runCmd := &cobra.Command{
		Use:                "run [flags] <source> <dest>",
		Short:              "Synchronize source to destination",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			defaults, err := config.DefaultConfig()
			if err != nil {
				return err
			}
			builder := config.NewBuilder(defaults)
			fs := pflag.NewFlagSet("run", pflag.ContinueOnError)
			cfg, remaining, warns, err := builder.Build(fs, args)
			if err != nil {
				return err
			}
			for _, w := range warns {
				logger.Warn(w)
			}
			if len(remaining) != 2 {
				fs.Usage()
				return fmt.Errorf("usage: lvmsync run [flags] <source> <dest>")
			}
			if cfg.DryRun {
				if err := estimateTransfer(remaining[0], cfg, logger); err != nil {
					return err
				}
				return nil
			}
			opts := RunOptions{
				DryRun:    cfg.DryRun,
				Transport: cfg.Transport,
				Delta:     cfg.Delta,
				DedupMode: cfg.DedupMode,
				BlockSize: cfg.BlockSize,
				CDCMin:    cfg.CDCMin,
				CDCAvg:    cfg.CDCAvg,
				CDCMax:    cfg.CDCMax,
				ChunkSeed: cfg.ChunkSeed,
				Output:    cfg.Output,
			}
			return r.Run(remaining[0], remaining[1], opts, logger)
		},
	}

	manifestCmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage manifests",
	}

	rebuildCmd := &cobra.Command{
		Use:                "rebuild [flags] <device>",
		Short:              "Rebuild manifest for device",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			defaults, err := config.DefaultConfig()
			if err != nil {
				return err
			}
			builder := config.NewBuilder(defaults)
			builder.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
			builder.FlagSets.Remote = pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
			builder.FlagSets.Compression = pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
			builder.FlagSets.LVM = pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
			builder.FlagSets.Transport = pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
			fs := pflag.NewFlagSet("rebuild", pflag.ContinueOnError)
			cfg, remaining, warns, err := builder.Build(fs, args)
			if err != nil {
				return err
			}
			for _, w := range warns {
				logger.Warn(w)
			}
			if len(remaining) != 1 {
				fs.Usage()
				return fmt.Errorf("usage: lvmsync manifest rebuild [flags] <device>")
			}
			return r.ManifestRebuild(remaining[0], cfg, logger)
		},
	}
	manifestCmd.AddCommand(rebuildCmd)

	verifyCmd := &cobra.Command{
		Use:                "verify [flags] <source> <dest>",
		Short:              "Verify that source and destination match",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.Verify(args, logger)
		},
	}

	rootCmd.AddCommand(runCmd, manifestCmd, verifyCmd)
	return rootCmd
}

// Execute runs the command tree with provided arguments.
// If args is nil, the global os.Args are used by cobra.
func Execute(args []string, logger *zap.Logger) error {
	return ExecuteWithRunner(args, logger, nil)
}

// ExecuteWithRunner runs the command tree using the provided Runner.
// If args is nil, the global os.Args are used by cobra.
func ExecuteWithRunner(args []string, logger *zap.Logger, r *Runner) error {
	if logger == nil {
		return fmt.Errorf("nil logger")
	}
	defer rootcmd.SyncLogger(logger)
	cmd := NewRootCmd(logger, r)
	if args != nil {
		cmd.SetArgs(args)
	}
	return cmd.Execute()
}

func estimateTransfer(src string, cfg *config.Config, logger *zap.Logger) error {
	if logger == nil {
		return fmt.Errorf("nil logger")
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	size := info.Size()
	// No manifest provided; estimate full size.
	if cfg.ManifestPath == "" {
		durMs, bwBps := transfer.Estimate(size, cfg.SpeedLimit)
		logger.Info("dry run",
			zap.Int64("size_bytes", size),
			zap.Int64("estimated_tx_bytes", size),
			zap.Int64("estimated_duration_ms", durMs),
			zap.Int64("estimated_bandwidth_bps", bwBps),
		)
		return nil
	}

	idx, err := manifest.Open(cfg.ManifestPath)
	if err != nil {
		return fmt.Errorf("open manifest: %w", err)
	}
	defer idx.Close()

	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer f.Close()

	chunks := idx.ChunkCount()
	samples := chunks
	if samples > 100 {
		samples = 100
	}
	step := uint64(1)
	if samples > 0 && chunks > samples {
		step = chunks / samples
	}

	changed := 0
	for i := uint64(0); i < samples; i++ {
		idxPos := i * step
		if idxPos >= chunks {
			idxPos = chunks - 1
		}
		off, length, flags, _, _, err := idx.Entry(idxPos)
		if err != nil {
			return fmt.Errorf("manifest entry: %w", err)
		}
		buf := make([]byte, length)
		n, err := f.ReadAt(buf, int64(off))
		if err != nil && err != io.EOF {
			return fmt.Errorf("read source: %w", err)
		}
		data := buf[:n]
		xx := xxh3.Hash(data)
		digest := blake3.Sum256(data)
		if !idx.Match(off, uint32(n), flags, xx, func() [32]byte { return digest }) {
			changed++
		}
		if err == io.EOF {
			break
		}
	}

	ratio := float64(changed)
	if samples > 0 {
		ratio /= float64(samples)
	}
	est := int64(ratio * float64(size))
	durMs, bwBps := transfer.Estimate(est, cfg.SpeedLimit)
	logger.Info(
		"dry run",
		zap.Int64("size_bytes", size),
		zap.Int64("estimated_tx_bytes", est),
		zap.Int64("estimated_duration_ms", durMs),
		zap.Int64("estimated_bandwidth_bps", bwBps),
	)
	return nil
}
