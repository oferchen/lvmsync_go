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

// startFunc allows tests to stub server startup.
var startFunc = func(ctx context.Context, opts Options, logger *zap.Logger) error {
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

func bindFlagSets(cmd *cobra.Command, v *viper.Viper) {
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

	v.BindPFlags(fs)
	v.SetEnvPrefix("LVMSYNC_GRPC")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
}

func loadConfig(v *viper.Viper) (Options, error) {
	cfgFile := v.GetString("config")
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("grpcd")
		v.AddConfigPath(".")
	}
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && cfgFile != "" {
			return Options{}, err
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
	}, nil
}

// NewCmd creates the root cobra command.
func NewCmd(logger *zap.Logger) *cobra.Command {
	v := viper.New()
	cmd := &cobra.Command{
		Use:   "lvmsync-grpcd",
		Short: "LVMSync gRPC daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := loadConfig(v)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return startFunc(ctx, opts, logger)
		},
	}
	bindFlagSets(cmd, v)
	return cmd
}

// Execute runs the command with provided args.
func Execute(args []string, logger *zap.Logger) error {
	cmd := NewCmd(logger)
	if args != nil {
		cmd.SetArgs(args)
	}
	return cmd.Execute()
}
