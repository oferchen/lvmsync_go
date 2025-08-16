package lvmsync

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	servecmd "lvmsync_go/cmd/serve"
	verifycmd "lvmsync_go/cmd/verify"
	"lvmsync_go/config"
	"lvmsync_go/manifest"
)

// RunOptions collects flags for the run command.
type RunOptions struct {
	DryRun    bool
	Transport string
	DedupMode string
	BlockSize int
	CDCMin    int
	CDCAvg    int
	CDCMax    int
	ChunkSeed uint64
}

var (
	// These function variables allow tests to stub command behavior.
	runCommand      = func(src, dst string, opts RunOptions, logger *zap.Logger) error { return nil }
	manifestRebuild = func(device string, dryRun bool, logger *zap.Logger) error { return nil }
	verifyRun       = func(args []string, logger *zap.Logger) error { return verifycmd.Run(args, logger) }
	serveRun        = func(args []string, logger *zap.Logger) error { return servecmd.Run(args, logger) }
)

// NewRootCmd creates the root cobra command with all subcommands wired.
func NewRootCmd(logger *zap.Logger) *cobra.Command {
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
			fs := pflag.NewFlagSet("run", pflag.ContinueOnError)
			flagSets := config.NewFlagSets(defaults)
			cfg, remaining, err := config.LoadConfig(flagSets, defaults, fs, args)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
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
				DedupMode: cfg.DedupMode,
				BlockSize: cfg.BlockSize,
				CDCMin:    cfg.CDCMin,
				CDCAvg:    cfg.CDCAvg,
				CDCMax:    cfg.CDCMax,
				ChunkSeed: cfg.ChunkSeed,
			}
			return runCommand(remaining[0], remaining[1], opts, logger)
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
			fs := pflag.NewFlagSet("rebuild", pflag.ContinueOnError)
			flagSets := config.NewFlagSets(defaults)
			flagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
			flagSets.Remote = pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
			flagSets.Compression = pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
			flagSets.LVM = pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
			flagSets.GRPC = pflag.NewFlagSet("gRPC Options", pflag.ExitOnError)
			flagSets.Transport = pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
			cfg, remaining, err := config.LoadConfig(flagSets, defaults, fs, args)
			if err != nil {
				return err
			}
			if len(remaining) != 1 {
				fs.Usage()
				return fmt.Errorf("usage: lvmsync manifest rebuild [flags] <device>")
			}
			return manifestRebuild(remaining[0], cfg.DryRun, logger)
		},
	}
	manifestCmd.AddCommand(rebuildCmd)

	verifyCmd := &cobra.Command{
		Use:                "verify [flags] <source> <dest>",
		Short:              "Verify that source and destination match",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return verifyRun(args, logger)
		},
	}

	serveCmd := &cobra.Command{
		Use:                "serve [flags]",
		Short:              "Run a transport listener",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serveRun(args, logger)
		},
	}

	rootCmd.AddCommand(runCmd, manifestCmd, verifyCmd, serveCmd)
	return rootCmd
}

// Execute runs the command tree with provided arguments.
// If args is nil, the global os.Args are used by cobra.
func Execute(args []string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	defer rootcmd.SyncLogger(logger)
	cmd := NewRootCmd(logger)
	if args != nil {
		cmd.SetArgs(args)
	}
	return cmd.Execute()
}

func estimateTransfer(src string, cfg *config.Config, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	size := info.Size()
	// No manifest provided; estimate full size.
	if cfg.ManifestPath == "" {
		var eta time.Duration
		if cfg.SpeedLimit > 0 {
			eta = time.Duration(size/int64(cfg.SpeedLimit)) * time.Second
		}
		logger.Info("dry run", zap.Int64("size_bytes", size), zap.Int64("estimated_tx_bytes", size), zap.Float64("eta_seconds", eta.Seconds()))
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
	var eta time.Duration
	if cfg.SpeedLimit > 0 {
		eta = time.Duration(est/int64(cfg.SpeedLimit)) * time.Second
	}
	logger.Info(
		"dry run",
		zap.Int64("size_bytes", size),
		zap.Int64("estimated_tx_bytes", est),
		zap.Float64("eta_seconds", eta.Seconds()),
	)
	return nil
}
