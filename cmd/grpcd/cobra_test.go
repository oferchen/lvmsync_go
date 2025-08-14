package main

import (
	"context"
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
