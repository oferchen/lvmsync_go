package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	grpcserver "lvmsync_go/grpc/server"
)

// Options holds configuration for the gRPC daemon.
type Options struct {
	GRPCPort         int
	TLSCert          string
	TLSKey           string
	CACert           string
	AllowInsecure    bool
	KeepaliveTime    time.Duration
	KeepaliveTimeout time.Duration
	RequestTimeout   time.Duration
}

func startServer(ctx context.Context, opts Options, logger *zap.Logger) error {
	addr := fmt.Sprintf(":%d", opts.GRPCPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	cfg := grpcserver.Config{
		TLSCert:          opts.TLSCert,
		TLSKey:           opts.TLSKey,
		CACert:           opts.CACert,
		AllowInsecure:    opts.AllowInsecure,
		KeepaliveTime:    opts.KeepaliveTime,
		KeepaliveTimeout: opts.KeepaliveTimeout,
		RequestTimeout:   opts.RequestTimeout,
	}
	srv, cleanup, err := grpcserver.New(cfg, nil, logger)
	if err != nil {
		ln.Close()
		return fmt.Errorf("init gRPC server: %w", err)
	}
	go func() {
		if err := srv.Serve(ln); err != nil {
			logger.Error("grpc serve", zap.Error(err))
		}
	}()
	<-ctx.Done()
	srv.GracefulStop()
	ln.Close()
	cleanup()
	return nil
}

// Runner holds dependencies for gRPC server execution.
type Runner struct {
	Start func(ctx context.Context, opts Options, logger *zap.Logger) error
}

// NewRunner returns a Runner with production dependencies.
func NewRunner() *Runner { return &Runner{Start: startServer} }

// NewRunnerWithDeps creates a Runner with custom start function for tests.
func NewRunnerWithDeps(start func(ctx context.Context, opts Options, logger *zap.Logger) error) *Runner {
	return &Runner{Start: start}
}

type flagBinder interface {
	BindPFlags(*pflag.FlagSet) error
	SetEnvPrefix(string)
	SetEnvKeyReplacer(*strings.Replacer)
	AutomaticEnv()
}

func bindFlagSets(cmd *cobra.Command, v flagBinder) error {
	general := pflag.NewFlagSet("General Options", pflag.ExitOnError)
	general.String("config", "", "config file")

	grpc := pflag.NewFlagSet("gRPC Options", pflag.ExitOnError)
	grpc.Int("grpc-port", 9443, "gRPC listen port")
	grpc.String("tls-cert", "", "TLS certificate file")
	grpc.String("tls-key", "", "TLS key file")
	grpc.String("ca-cert", "", "CA certificate file")
	grpc.Bool("allow-insecure", false, "allow plaintext gRPC")
	grpc.Duration("keepalive-time", 2*time.Minute, "interval between server pings")
	grpc.Duration("keepalive-timeout", 20*time.Second, "timeout waiting for keepalive ack")
	grpc.Duration("request-timeout", 15*time.Second, "deadline for unary RPCs")

	fs := cmd.Flags()
	fs.AddFlagSet(general)
	fs.AddFlagSet(grpc)

	if err := v.BindPFlags(fs); err != nil {
		return err
	}
	v.SetEnvPrefix("LVMSYNC_GRPC")
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
		v.SetConfigName("grpcd")
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
		GRPCPort:         v.GetInt("grpc-port"),
		TLSCert:          v.GetString("tls-cert"),
		TLSKey:           v.GetString("tls-key"),
		CACert:           v.GetString("ca-cert"),
		AllowInsecure:    v.GetBool("allow-insecure"),
		KeepaliveTime:    v.GetDuration("keepalive-time"),
		KeepaliveTimeout: v.GetDuration("keepalive-timeout"),
		RequestTimeout:   v.GetDuration("request-timeout"),
	}, warns, nil
}

// NewCmd creates the root cobra command.
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
		Use:   "lvmsync-grpcd",
		Short: "LVMSync gRPC daemon",
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
			return r.Start(ctx, opts, logger)
		},
	}
	if err := bindFlagSets(cmd, v); err != nil {
		return nil, err
	}
	return cmd, nil
}

// Execute runs the command with provided args.
func (r *Runner) Execute(args []string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	defer rootcmd.SyncLogger(logger)
	cmd, err := r.NewCmd(logger, nil)
	if err != nil {
		return err
	}
	if args != nil {
		cmd.SetArgs(args)
	}
	return cmd.Execute()
}
