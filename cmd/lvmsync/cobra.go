package lvmsync

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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
	verifyCommand   = func(src, dst string) error { return nil }
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
		Use:   "verify <source> <dest>",
		Short: "Verify that source and destination match",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return verifyCommand(args[0], args[1])
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
