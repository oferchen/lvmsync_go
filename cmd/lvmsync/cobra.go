package lvmsync

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	verifycmd "lvmsync_go/cmd/verify"
	"lvmsync_go/config"
)

// RunOptions collects flags for the run command.
type RunOptions struct {
	DryRun    bool
	Transport string
}

var (
	// These function variables allow tests to stub command behavior.
	runCommand      = func(src, dst string, opts RunOptions) error { return nil }
	manifestRebuild = func(device string, dryRun bool) error { return nil }
	verifyRun       = func(args []string) error { return verifycmd.Run(args, nil) }
)

// NewRootCmd creates the root cobra command with all subcommands wired.
func NewRootCmd() *cobra.Command {
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
			if len(remaining) != 2 {
				fs.Usage()
				return fmt.Errorf("usage: lvmsync run [flags] <source> <dest>")
			}
			if cfg.DryRun {
				logger := zap.NewExample()
				defer logger.Sync()
				if err := estimateTransfer(remaining[0], cfg, logger); err != nil {
					return err
				}
				return nil
			}
			opts := RunOptions{
				DryRun:    cfg.DryRun,
				Transport: cfg.Transport,
			}
			return runCommand(remaining[0], remaining[1], opts)
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
			cfg, remaining, err := config.LoadConfig(flagSets, defaults, fs, args)
			if err != nil {
				return err
			}
			if len(remaining) != 1 {
				fs.Usage()
				return fmt.Errorf("usage: lvmsync manifest rebuild [flags] <device>")
			}
			return manifestRebuild(remaining[0], cfg.DryRun)
		},
	}
	manifestCmd.AddCommand(rebuildCmd)

	verifyCmd := &cobra.Command{
		Use:                "verify [flags] <source> <dest>",
		Short:              "Verify that source and destination match",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return verifyRun(args)
		},
	}

	rootCmd.AddCommand(runCmd, manifestCmd, verifyCmd)
	return rootCmd
}

// Execute runs the command tree with provided arguments.
// If args is nil, the global os.Args are used by cobra.
func Execute(args []string) error {
	cmd := NewRootCmd()
	if args != nil {
		cmd.SetArgs(args)
	}
	return cmd.Execute()
}

func estimateTransfer(src string, cfg *config.Config, logger *zap.Logger) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	size := info.Size()
	var eta time.Duration
	if cfg.SpeedLimit > 0 {
		eta = time.Duration(size/int64(cfg.SpeedLimit)) * time.Second
	}
	logger.Info("dry run", zap.Int64("size_bytes", size), zap.Duration("eta", eta))
	return nil
}
