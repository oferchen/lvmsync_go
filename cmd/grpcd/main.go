package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	grpcserver "lvmsync_go/grpc/server"
	lvmagent "lvmsync_go/internal/lvm"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

var (
	newZapProduction = zap.NewProduction
	exitFunc         = os.Exit
)

func main() {
	logger, err := newZapProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		exitFunc(1)
	}
	zap.ReplaceGlobals(logger)
	defer syncAndExit(logger)

	port := getEnvInt("GRPC_PORT", 8443)
	tlsCert := getEnv("TLS_CERT", "")
	tlsKey := getEnv("TLS_KEY", "")
	caCert := getEnv("CA_CERT", "")
	allowInsecure := getEnvBool("ALLOW_INSECURE", false)

	pflag.IntVar(&port, "grpc-port", port, "gRPC port to listen on")
	pflag.StringVar(&tlsCert, "tls-cert", tlsCert, "TLS certificate file")
	pflag.StringVar(&tlsKey, "tls-key", tlsKey, "TLS key file")
	pflag.StringVar(&caCert, "ca-cert", caCert, "CA certificate file")
	pflag.BoolVar(&allowInsecure, "allow-insecure", allowInsecure, "allow insecure (no TLS)")
	pflag.Parse()

	cfg := grpcserver.Config{
		TLSCert:       tlsCert,
		TLSKey:        tlsKey,
		CACert:        caCert,
		AllowInsecure: allowInsecure,
	}

	agent := lvmagent.NewSudoAgent("", nil)
	srv := grpcserver.New(cfg, agent)

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

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
