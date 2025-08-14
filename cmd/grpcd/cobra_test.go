package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestFlagParsing(t *testing.T) {
	logger := zap.NewNop()
	var got Options
	orig := startFunc
	startFunc = func(ctx context.Context, opts Options, _ *zap.Logger) error {
		got = opts
		return nil
	}
	defer func() { startFunc = orig }()
	args := []string{
		"--grpc-port", "1234",
		"--tls-cert", "cert",
		"--tls-key", "key",
		"--ca-cert", "ca",
		"--allow-insecure",
	}
	if err := Execute(args, logger); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := Options{GRPCPort: 1234, TLSCert: "cert", TLSKey: "key", CACert: "ca", AllowInsecure: true}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestGRPCPortPrecedence(t *testing.T) {
	logger := zap.NewNop()
	cfgPath := writeTempConfig(t, "grpc-port: 1111\n")
	t.Setenv("LVMSYNC_GRPC_GRPC_PORT", "2222")

	var got Options
	orig := startFunc
	startFunc = func(ctx context.Context, opts Options, _ *zap.Logger) error {
		got = opts
		return nil
	}
	defer func() { startFunc = orig }()

	args := []string{"--config", cfgPath, "--grpc-port", "3333"}
	if err := Execute(args, logger); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.GRPCPort != 3333 {
		t.Fatalf("expected 3333, got %d", got.GRPCPort)
	}

	got = Options{}
	if err := Execute([]string{"--config", cfgPath}, logger); err != nil {
		t.Fatalf("Execute env: %v", err)
	}
	if got.GRPCPort != 2222 {
		t.Fatalf("expected 2222, got %d", got.GRPCPort)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
