package lvmsync

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
func newViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("LVMSYNC")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	return v
}

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "lvmsync",
		Short: "LVMSync command line tool",
	}
	runV := newViper()
	runCmd := &cobra.Command{
		Use:   "run <source> <dest>",
		Short: "Synchronize source to destination",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := RunOptions{
				DryRun:    runV.GetBool("dry-run"),
				Transport: runV.GetString("transport"),
			}
			return runCommand(args[0], args[1], opts)
		},
	}
	runCmd.Flags().Bool("dry-run", false, "print actions without executing")
	runCmd.Flags().String("transport", "", "ordered transports to try (e.g. 'tcp+tls,ssh')")
	runV.BindPFlags(runCmd.Flags())

	manifestCmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage manifests",
	}
	rebuildV := newViper()
	rebuildCmd := &cobra.Command{
		Use:   "rebuild <device>",
		Short: "Rebuild manifest for device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return manifestRebuild(args[0], rebuildV.GetBool("dry-run"))
		},
	}
	rebuildCmd.Flags().Bool("dry-run", false, "print actions without executing")
	rebuildV.BindPFlags(rebuildCmd.Flags())
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
