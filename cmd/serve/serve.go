package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

const deprecationMsg = "serve command deprecated; use lvmsyncd instead"

func newCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Deprecated server command",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return errors.New(deprecationMsg)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), deprecationMsg)
	})
	return cmd
}

func run(args []string, w io.Writer) int {
	cmd := newCommand()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(w, deprecationMsg)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}
