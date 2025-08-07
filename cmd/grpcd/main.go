package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	grpcserver "lvmsync_go/grpc/server"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	zap.ReplaceGlobals(logger)
	defer func() { _ = logger.Sync() }()

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

	srv := grpcserver.New(cfg, nil)

	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		zap.L().Fatal("listen", zap.Error(err))
	}

	zap.L().Info("gRPC server listening", zap.Int("port", port))
	if err := srv.Serve(lis); err != nil {
		zap.L().Fatal("serve", zap.Error(err))
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
