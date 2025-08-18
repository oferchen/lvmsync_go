package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func TestFlagParsing(t *testing.T) {
	logger := zap.NewNop()
	var got Options
	runner := NewRunnerWithDeps(func(ctx context.Context, opts Options, _ *zap.Logger) error {
		got = opts
		return nil
	})
	args := []string{
		"--grpc-port", "1234",
		"--tls-cert", "cert",
		"--tls-key", "key",
		"--ca-cert", "ca",
		"--allow-insecure",
		"--keepalive-time", "30s",
		"--keepalive-timeout", "5s",
		"--request-timeout", "1m",
	}
	if err := runner.Execute(args, logger); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := Options{GRPCPort: 1234, TLSCert: "cert", TLSKey: "key", CACert: "ca", AllowInsecure: true, KeepaliveTime: 30 * time.Second, KeepaliveTimeout: 5 * time.Second, RequestTimeout: time.Minute}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestGRPCPortPrecedence(t *testing.T) {
	logger := zap.NewNop()
	cfgPath := writeTempConfig(t, "grpc-port: 1111\n")
	t.Setenv("LVMSYNC_GRPC_GRPC_PORT", "2222")

	var got Options
	runner := NewRunnerWithDeps(func(ctx context.Context, opts Options, _ *zap.Logger) error {
		got = opts
		return nil
	})

	args := []string{"--config", cfgPath, "--grpc-port", "3333"}
	if err := runner.Execute(args, logger); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.GRPCPort != 3333 {
		t.Fatalf("expected 3333, got %d", got.GRPCPort)
	}

	got = Options{}
	if err := runner.Execute([]string{"--config", cfgPath}, logger); err != nil {
		t.Fatalf("Execute env: %v", err)
	}
	if got.GRPCPort != 2222 {
		t.Fatalf("expected 2222, got %d", got.GRPCPort)
	}
}

type bindErrViper struct {
	*viper.Viper
}

func (b *bindErrViper) BindPFlags(_ *pflag.FlagSet) error { return errors.New("bind fail") }
func (b *bindErrViper) Underlying() *viper.Viper          { return b.Viper }

func TestNewCmdBindError(t *testing.T) {
	r := NewRunner()
	if _, err := r.NewCmd(zap.NewNop(), &bindErrViper{Viper: viper.New()}); err == nil || err.Error() != "bind fail" {
		t.Fatalf("expected bind fail, got %v", err)
	}
}

func TestLoadConfigUnknownKey(t *testing.T) {
	v := viper.New()
	if err := bindFlagSets(&cobra.Command{}, v); err != nil {
		t.Fatalf("bindFlagSets: %v", err)
	}
	cfgPath := writeTempConfig(t, "extra: 1\n")
	v.Set("config", cfgPath)
	_, warns, err := loadConfig(v)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(warns) != 1 || warns[0] != `unknown configuration key "extra"` {
		t.Fatalf("unexpected warnings %v", warns)
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
