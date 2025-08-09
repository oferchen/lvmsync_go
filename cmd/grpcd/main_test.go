package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type failCore struct{}

func (failCore) Enabled(zapcore.Level) bool        { return false }
func (failCore) With([]zapcore.Field) zapcore.Core { return failCore{} }
func (failCore) Check(zapcore.Entry, *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return nil
}
func (failCore) Write(zapcore.Entry, []zapcore.Field) error { return nil }
func (failCore) Sync() error                                { return errors.New("sync failure") }

func TestSyncAndExitLogsError(t *testing.T) {
	logger := zap.New(failCore{})

	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()

	var code int
	exitFunc = func(c int) { code = c }

	syncAndExit(logger)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestInitConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "grpcd.yaml")
	cfg := []byte("grpc-port: 1111\nallow-insecure: false\n")
	if err := os.WriteFile(cfgFile, cfg, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Run("config file", func(t *testing.T) {
		v, err := initConfig([]string{"--config", cfgFile})
		if err != nil {
			t.Fatalf("initConfig: %v", err)
		}
		if port := v.GetInt("grpc-port"); port != 1111 {
			t.Fatalf("expected port 1111, got %d", port)
		}
		if v.GetBool("allow-insecure") {
			t.Fatalf("expected allow-insecure false")
		}
	})

	t.Run("env overrides", func(t *testing.T) {
		t.Setenv("LVMSYNC_GRPC_GRPC_PORT", "2222")
		t.Setenv("LVMSYNC_GRPC_ALLOW_INSECURE", "true")
		v, err := initConfig([]string{"--config", cfgFile})
		if err != nil {
			t.Fatalf("initConfig: %v", err)
		}
		if port := v.GetInt("grpc-port"); port != 2222 {
			t.Fatalf("expected port 2222, got %d", port)
		}
		if !v.GetBool("allow-insecure") {
			t.Fatalf("expected allow-insecure true")
		}
	})

	t.Run("flag overrides", func(t *testing.T) {
		t.Setenv("LVMSYNC_GRPC_GRPC_PORT", "2222")
		t.Setenv("LVMSYNC_GRPC_ALLOW_INSECURE", "true")
		v, err := initConfig([]string{"--config", cfgFile, "--grpc-port", "3333", "--allow-insecure=false"})
		if err != nil {
			t.Fatalf("initConfig: %v", err)
		}
		if port := v.GetInt("grpc-port"); port != 3333 {
			t.Fatalf("expected port 3333, got %d", port)
		}
		if v.GetBool("allow-insecure") {
			t.Fatalf("expected allow-insecure false")
		}
	})
}
