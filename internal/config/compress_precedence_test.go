package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestCompressFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("compress: none\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_COMPRESSION_COMPRESS", "lz4")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile, "--compress", "zstd"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.Compress != "zstd" {
		t.Fatalf("Compress=%q want %q", cfg.Compress, "zstd")
	}
}

func TestCompressEnvOverridesYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("compress: none\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_COMPRESSION_COMPRESS", "lz4")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.Compress != "lz4" {
		t.Fatalf("Compress=%q want %q", cfg.Compress, "lz4")
	}
}
