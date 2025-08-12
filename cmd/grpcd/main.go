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
	"google.golang.org/grpc"

	grpcserver "lvmsync_go/grpc/server"
	lvmagent "lvmsync_go/internal/lvm"
)

var (
	newZapProduction = zap.NewProduction
	exitFunc         = os.Exit
	newServer        = grpcserver.New
	listen           = net.Listen
	serve            = func(s *grpc.Server, lis net.Listener) error { return s.Serve(lis) }
)

func main() {
	logger, err := newZapProduction()
	if err != nil {
		zap.NewNop().Error("init logger", zap.Error(err))
		exitFunc(1)
		return
	}
	defer syncLogger(logger)

	fatal := func(msg string, err error) {
		logger.Error(msg, zap.Error(err))
		syncLogger(logger)
		exitFunc(1)
	}

	v, err := initConfig(os.Args[1:])
	if err != nil {
		fatal("init config", err)
		syncLogger(logger)
		exitFunc(1)
		return
	}

	cfg := grpcserver.Config{
		TLSCert:       v.GetString("tls-cert"),
		TLSKey:        v.GetString("tls-key"),
		CACert:        v.GetString("ca-cert"),
		AllowInsecure: v.GetBool("allow-insecure"),
	}

	agent := lvmagent.NewAgent(nil, nil)
	srv, srvErr := newServer(cfg, agent)
	if srvErr != nil {
		fatal("init gRPC server", srvErr)
		return
	}

	port := v.GetInt("grpc-port")
	lis, listenErr := listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if listenErr != nil {
		fatal("listen", listenErr)
		return
	}

	logger.Info("gRPC server listening", zap.Int("port", port))
	if serveErr := srv.Serve(lis); serveErr != nil {
		fatal("serve", serveErr)
		return
	}
}

// syncLogger flushes any buffered log entries and logs if the sync fails.
func syncLogger(logger *zap.Logger) {
	if err := logger.Sync(); err != nil {
		logger.Error("Logger sync error", zap.Error(err))
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
