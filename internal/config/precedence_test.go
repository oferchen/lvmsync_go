package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestFreezeTimeoutFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("freeze-timeout: 1s\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_FREEZE_TIMEOUT", "2s")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile, "--freeze-timeout", "3s"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.FreezeTimeout != 3*time.Second {
		t.Fatalf("FreezeTimeout=%v want %v", cfg.FreezeTimeout, 3*time.Second)
	}
}

func TestFreezeTimeoutEnvOverridesYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("freeze-timeout: 1s\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_FREEZE_TIMEOUT", "2s")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.FreezeTimeout != 2*time.Second {
		t.Fatalf("FreezeTimeout=%v want %v", cfg.FreezeTimeout, 2*time.Second)
	}
}

func TestLVMSyncPathFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("lvmsync_path: yaml\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_LVMSYNC_PATH", "env")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile, "--lvmsync-path", "flag"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.LVMSyncPath != "flag" {
		t.Fatalf("LVMSyncPath=%q want %q", cfg.LVMSyncPath, "flag")
	}
}

func TestLVMSyncPathEnvOverridesYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("lvmsync_path: yaml\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_LVMSYNC_PATH", "env")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.LVMSyncPath != "env" {
		t.Fatalf("LVMSyncPath=%q want %q", cfg.LVMSyncPath, "env")
	}
}

func TestStrictHostKeyCheckFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("strict_host_key_checking: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_STRICT_HOST_KEY_CHECKING", "false")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile, "--strict-host-key-checking", "true"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !cfg.StrictHostKeyCheck {
		t.Fatalf("StrictHostKeyCheck=%v want %v", cfg.StrictHostKeyCheck, true)
	}
}

func TestStrictHostKeyCheckEnvOverridesYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("strict_host_key_checking: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_STRICT_HOST_KEY_CHECKING", "false")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.StrictHostKeyCheck {
		t.Fatalf("StrictHostKeyCheck=%v want %v", cfg.StrictHostKeyCheck, false)
	}
}

func TestCompressThresholdFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("compress_threshold: 0.5\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_COMPRESSION_COMPRESS_THRESHOLD", "0.6")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile, "--compress-threshold", "0.7"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.CompressThreshold != 0.7 {
		t.Fatalf("CompressThreshold=%v want %v", cfg.CompressThreshold, 0.7)
	}
}

func TestCompressThresholdEnvOverridesYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("compress_threshold: 0.5\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_COMPRESSION_COMPRESS_THRESHOLD", "0.6")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.CompressThreshold != 0.6 {
		t.Fatalf("CompressThreshold=%v want %v", cfg.CompressThreshold, 0.6)
	}
}
