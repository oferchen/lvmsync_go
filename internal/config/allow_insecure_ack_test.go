package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestAllowInsecureRequiresFlagOrEnv(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	builder := NewBuilder(defaults)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("allow_insecure: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	os.Unsetenv("LVMSYNC_ALLOW_INSECURE")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	if _, _, _, err := builder.Build(fs, []string{"--config", cfgPath}); err == nil {
		t.Fatalf("expected error without explicit acknowledgment")
	}

	t.Setenv("LVMSYNC_ALLOW_INSECURE", "1")
	fs = pflag.NewFlagSet("test", pflag.ContinueOnError)
	if _, _, _, err := builder.Build(fs, []string{"--config", cfgPath}); err != nil {
		t.Fatalf("unexpected error with env ack: %v", err)
	}

	fs = pflag.NewFlagSet("test", pflag.ContinueOnError)
	os.Unsetenv("LVMSYNC_ALLOW_INSECURE")
	if _, _, _, err := builder.Build(fs, []string{"--config", cfgPath, "--allow-insecure"}); err != nil {
		t.Fatalf("unexpected error with flag ack: %v", err)
	}

	t.Run("sets AllowInsecure with env or flag", func(t *testing.T) {
		t.Setenv("LVMSYNC_ALLOW_INSECURE", "1")
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		if cfg, _, _, err := builder.Build(fs, []string{"--config", cfgPath}); err != nil {
			t.Fatalf("unexpected error with env ack: %v", err)
		} else if !cfg.AllowInsecure {
			t.Fatalf("AllowInsecure not set with env ack")
		}

		os.Unsetenv("LVMSYNC_ALLOW_INSECURE")
		fs = pflag.NewFlagSet("test", pflag.ContinueOnError)
		if cfg, _, _, err := builder.Build(fs, []string{"--config", cfgPath, "--allow-insecure"}); err != nil {
			t.Fatalf("unexpected error with flag ack: %v", err)
		} else if !cfg.AllowInsecure {
			t.Fatalf("AllowInsecure not set with flag ack")
		}
	})
}
