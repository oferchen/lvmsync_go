package main

import (
	"context"
	"fmt"
	"plugin"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Options holds configuration for lvmsyncd.
type Options struct {
	Modules    []string
	Listen     []string
	Once       bool
	SudoHelper string
}

func loadModules(mods []string, logger *zap.Logger) error {
	for _, m := range mods {
		if m == "" {
			continue
		}
		if _, err := plugin.Open(m); err != nil {
			return fmt.Errorf("load module %q: %w", m, err)
		}
		if logger != nil {
			logger.Info("module_loaded", zap.String("path", m))
		}
	}
	return nil
}

func start(ctx context.Context, opts Options, logger *zap.Logger) error {
	if err := loadModules(opts.Modules, logger); err != nil {
		return err
	}
	for _, uri := range opts.Listen {
		logger.Info("listen_uri", zap.String("uri", uri))
	}
	if opts.SudoHelper != "" {
		logger.Info("sudo_helper", zap.String("path", opts.SudoHelper))
	}
	if opts.Once {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

// Runner holds dependencies for lvmsyncd execution.
type Runner struct {
	Start func(ctx context.Context, opts Options, logger *zap.Logger) error
}

// NewRunner returns a Runner with production dependencies.
func NewRunner() *Runner { return &Runner{Start: start} }

// NewRunnerWithDeps creates a Runner with custom start function for tests.
func NewRunnerWithDeps(start func(ctx context.Context, opts Options, logger *zap.Logger) error) *Runner {
	return &Runner{Start: start}
}

func bindFlagSets(cmd *cobra.Command, v *viper.Viper) {
	daemon := pflag.NewFlagSet("Daemon Options", pflag.ExitOnError)
	daemon.StringSlice("listen", nil, "listen URI (repeatable)")
	daemon.StringSlice("module", nil, "module path to load (repeatable)")
	daemon.Bool("once", false, "run once and exit")
	daemon.String("sudo-helper", "", "optional sudo helper path")

	fs := cmd.Flags()
	fs.AddFlagSet(daemon)

	v.BindPFlags(fs)
	v.SetEnvPrefix("LVMSYNC_DAEMON")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
}

func loadConfig(v *viper.Viper) (Options, error) {
	cfgFile := v.GetString("config")
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("lvmsyncd")
		v.AddConfigPath(".")
	}
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && cfgFile != "" {
			return Options{}, err
		}
	}
	return Options{
		Modules:    v.GetStringSlice("module"),
		Listen:     v.GetStringSlice("listen"),
		Once:       v.GetBool("once"),
		SudoHelper: v.GetString("sudo-helper"),
	}, nil
}

// NewCmd creates the lvmsyncd cobra command.
func (r *Runner) NewCmd(logger *zap.Logger) *cobra.Command {
	v := viper.New()
	cmd := &cobra.Command{
		Use:   "lvmsyncd",
		Short: "LVMSync daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := loadConfig(v)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return r.Start(ctx, opts, logger)
		},
	}
	bindFlagSets(cmd, v)
	return cmd
}

// Execute runs the command with provided args.
func (r *Runner) Execute(args []string, logger *zap.Logger) error {
	cmd := r.NewCmd(logger)
	if args != nil {
		cmd.SetArgs(args)
	}
	return cmd.Execute()
}
