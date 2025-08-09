package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	grpcserver "lvmsync_go/grpc/server"
	lvmagent "lvmsync_go/internal/lvm"
)

var (
	newZapProduction = zap.NewProduction
	exitFunc         = os.Exit
)

func main() {
	logger, err := newZapProduction()
	if err != nil {
		zap.NewNop().Error("init logger", zap.Error(err))
		exitFunc(1)
	}
	zap.ReplaceGlobals(logger)
	defer syncAndExit(logger)

	v, err := initConfig(os.Args[1:])
	if err != nil {
		zap.L().Error("init config", zap.Error(err))
		exitFunc(1)
	}

	cfg := grpcserver.Config{
		TLSCert:       v.GetString("tls-cert"),
		TLSKey:        v.GetString("tls-key"),
		CACert:        v.GetString("ca-cert"),
		AllowInsecure: v.GetBool("allow-insecure"),
	}

	agent := lvmagent.NewSudoAgent("", nil)
	srv, srvErr := grpcserver.New(cfg, agent)
	if srvErr != nil {
		zap.L().Fatal("init gRPC server", zap.Error(srvErr))
	}

	port := v.GetInt("grpc-port")
	lis, listenErr := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if listenErr != nil {
		zap.L().Fatal("listen", zap.Error(listenErr))
	}

	zap.L().Info("gRPC server listening", zap.Int("port", port))
	if serveErr := srv.Serve(lis); serveErr != nil {
		zap.L().Fatal("serve", zap.Error(serveErr))
	}
}

func syncAndExit(logger *zap.Logger) {
	if syncErr := logger.Sync(); syncErr != nil {
		logger.Error("failed to sync logger", zap.Error(syncErr))
		exitFunc(1)
	}
}

func initConfig(args []string) (*viper.Viper, error) {
	tlsFlags := pflag.NewFlagSet("tls", pflag.ContinueOnError)
	tlsFlags.String("tls-cert", "", "TLS certificate file")
	tlsFlags.String("tls-key", "", "TLS key file")
	tlsFlags.String("ca-cert", "", "CA certificate file")
	tlsFlags.Bool("allow-insecure", false, "allow insecure (no TLS)")

	netFlags := pflag.NewFlagSet("network", pflag.ContinueOnError)
	netFlags.Int("grpc-port", 8443, "gRPC port to listen on")

	fs := pflag.NewFlagSet("grpcd", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.AddFlagSet(netFlags)
	fs.AddFlagSet(tlsFlags)
	fs.String("config", "", "configuration file")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	v := viper.New()
	if err := v.BindPFlags(fs); err != nil {
		return nil, err
	}
	v.SetEnvPrefix("LVMSYNC_GRPC")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	if cfgFile := v.GetString("config"); cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("grpcd")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		var cfgNotFound viper.ConfigFileNotFoundError
		if !errors.As(err, &cfgNotFound) {
			return nil, err
		}
	}

	return v, nil
}
