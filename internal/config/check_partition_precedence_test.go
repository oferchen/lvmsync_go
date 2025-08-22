package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestCheckPartitionFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	flagSets := NewFlagSets(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	registerFlags(flagSets, fs)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("check_partition: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_CHECK_PARTITION", "false")
	if err := fs.Parse([]string{"--config", cfgFile, "--check-partition"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	v, _, _, _, err := buildViper(flagSets)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	vb := &builder{v: v, defaults: defaults}
	cfg, err := vb.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !cfg.CheckPartition {
		t.Fatalf("CheckPartition=%v want %v", cfg.CheckPartition, true)
	}
}

func TestCheckPartitionEnvOverridesYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	flagSets := NewFlagSets(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	registerFlags(flagSets, fs)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("check_partition: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_CHECK_PARTITION", "false")
	if err := fs.Parse([]string{"--config", cfgFile}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	v, _, _, _, err := buildViper(flagSets)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	vb := &builder{v: v, defaults: defaults}
	cfg, err := vb.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.CheckPartition {
		t.Fatalf("CheckPartition=%v want %v", cfg.CheckPartition, false)
	}
}
