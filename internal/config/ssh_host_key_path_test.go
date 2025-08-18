package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestFlagsOverrideEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("ssh_host_key_path: yaml\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_SSH_HOST_KEY_PATH", "env")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile, "--ssh-host-key-path", "flag"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.SSHHostKeyPath != "flag" {
		t.Fatalf("SSHHostKeyPath=%q want %q", cfg.SSHHostKeyPath, "flag")
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("ssh_host_key_path: yaml\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_SSH_HOST_KEY_PATH", "env")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.SSHHostKeyPath != "env" {
		t.Fatalf("SSHHostKeyPath=%q want %q", cfg.SSHHostKeyPath, "env")
	}
}

func TestUnknownKeysProduceWarnings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("bogus: value\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, _, warns, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(warns) != 1 || warns[0] != "unknown configuration key \"bogus\"" {
		t.Fatalf("warnings=%v", warns)
	}
}
