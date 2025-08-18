package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"plugin"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"lvmsync_go/transport"
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

func start(ctx context.Context, opts Options, logger *zap.Logger, getTransport func(string, transport.Config) (transport.Interface, error)) error {
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

	g, ctx := errgroup.WithContext(ctx)

	for _, uri := range opts.Listen {
		u, err := url.Parse(uri)
		if err != nil {
			return fmt.Errorf("parse listen uri %q: %w", uri, err)
		}
		scheme := u.Scheme
		addr := u.Host
		switch scheme {
		case "grpc":
			g.Go(func() error {
				ln, err := net.Listen("tcp", addr)
				if err != nil {
					return fmt.Errorf("grpc listen %q: %w", addr, err)
				}
				srv := grpc.NewServer()
				serveErr := make(chan error, 1)
				go func() { serveErr <- srv.Serve(ln) }()
				select {
				case err := <-serveErr:
					if errors.Is(err, grpc.ErrServerStopped) || errors.Is(err, net.ErrClosed) {
						return ctx.Err()
					}
					return fmt.Errorf("grpc serve: %w", err)
				case <-ctx.Done():
					srv.GracefulStop()
					ln.Close()
					err := <-serveErr
					if errors.Is(err, grpc.ErrServerStopped) || errors.Is(err, net.ErrClosed) {
						return ctx.Err()
					}
					if err != nil {
						return fmt.Errorf("grpc serve: %w", err)
					}
					return ctx.Err()
				}
			})
		default:
			schemeCopy := scheme
			addrCopy := addr
			uriCopy := uri
			g.Go(func() error {
				tr, err := getTransport(schemeCopy, transport.Config{Logger: logger})
				if err != nil {
					return fmt.Errorf("get transport %q: %w", schemeCopy, err)
				}
				ln, err := tr.Listen(ctx, addrCopy)
				if err != nil {
					return fmt.Errorf("listen %q: %w", uriCopy, err)
				}
				defer ln.Close()
				go func() {
					<-ctx.Done()
					ln.Close()
				}()
				for {
					if _, err := ln.Accept(); err != nil {
						select {
						case <-ctx.Done():
							return ctx.Err()
						default:
							return err
						}
					}
				}
			})
		}
	}

	return g.Wait()
}

// Runner holds dependencies for lvmsyncd execution.
type Runner struct {
	Start        func(ctx context.Context, opts Options, logger *zap.Logger, getTransport func(string, transport.Config) (transport.Interface, error)) error
	GetTransport func(string, transport.Config) (transport.Interface, error)
}

// NewRunner returns a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{
		Start:        start,
		GetTransport: transport.Get,
	}
}

// NewRunnerWithDeps creates a Runner with custom dependencies for tests.
func NewRunnerWithDeps(
	startFn func(ctx context.Context, opts Options, logger *zap.Logger, getTransport func(string, transport.Config) (transport.Interface, error)) error,
	get func(string, transport.Config) (transport.Interface, error),
) *Runner {
	r := NewRunner()
	if startFn != nil {
		r.Start = startFn
	}
	if get != nil {
		r.GetTransport = get
	}
	return r
}

type flagBinder interface {
	BindPFlags(*pflag.FlagSet) error
	SetEnvPrefix(string)
	SetEnvKeyReplacer(*strings.Replacer)
	AutomaticEnv()
}

func bindFlagSets(cmd *cobra.Command, v flagBinder) error {
	daemon := pflag.NewFlagSet("Daemon Options", pflag.ExitOnError)
	daemon.StringSlice("listen", nil, "listen URI (repeatable)")
	daemon.StringSlice("module", nil, "module path to load (repeatable)")
	daemon.Bool("once", false, "run once and exit")
	daemon.String("sudo-helper", "", "optional sudo helper path")

	fs := cmd.Flags()
	fs.AddFlagSet(daemon)

	if err := v.BindPFlags(fs); err != nil {
		return err
	}
	v.SetEnvPrefix("LVMSYNC_DAEMON")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	return nil
}

func loadConfig(v *viper.Viper) (Options, []string, error) {
	known := map[string]struct{}{}
	for _, k := range v.AllKeys() {
		known[k] = struct{}{}
	}
	cfgFile := v.GetString("config")
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("lvmsyncd")
		v.AddConfigPath(".")
	}
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && cfgFile != "" {
			return Options{}, nil, err
		}
	}
	var warns []string
	for _, k := range v.AllKeys() {
		if _, ok := known[k]; !ok {
			warns = append(warns, fmt.Sprintf("unknown configuration key %q", k))
		}
	}
	return Options{
		Modules:    v.GetStringSlice("module"),
		Listen:     v.GetStringSlice("listen"),
		Once:       v.GetBool("once"),
		SudoHelper: v.GetString("sudo-helper"),
	}, warns, nil
}

// NewCmd creates the lvmsyncd cobra command.
func (r *Runner) NewCmd(logger *zap.Logger, v flagBinder) (*cobra.Command, error) {
	if v == nil {
		v = viper.New()
	}
	vv, ok := v.(*viper.Viper)
	if !ok {
		type viperGetter interface{ Underlying() *viper.Viper }
		getter, ok := v.(viperGetter)
		if !ok {
			return nil, fmt.Errorf("invalid viper binder")
		}
		vv = getter.Underlying()
	}
	cmd := &cobra.Command{
		Use:   "lvmsyncd",
		Short: "LVMSync daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, warns, err := loadConfig(vv)
			if err != nil {
				return err
			}
			for _, w := range warns {
				if logger != nil {
					logger.Warn(w)
				}
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return r.Start(ctx, opts, logger, r.GetTransport)
		},
	}
	if err := bindFlagSets(cmd, v); err != nil {
		return nil, err
	}
	return cmd, nil
}

// Execute runs the command with provided args.
func (r *Runner) Execute(args []string, logger *zap.Logger) error {
	cmd, err := r.NewCmd(logger, nil)
	if err != nil {
		return err
	}
	if args != nil {
		cmd.SetArgs(args)
	}
	return cmd.Execute()
}
