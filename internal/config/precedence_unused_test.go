package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestFlagOverridesEnvAndYAMLWithUnusedKeyWarn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	// Remove SSH flag bindings so ssh_host becomes unbound and emits a warning.
	b.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("freeze-timeout: 1s\nssh_host: example\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_FREEZE_TIMEOUT", "2s")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, warns, err := b.Build(fs, []string{"--config", cfgFile, "--freeze-timeout", "3s"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.FreezeTimeout != 3*time.Second {
		t.Fatalf("FreezeTimeout=%v want %v", cfg.FreezeTimeout, 3*time.Second)
	}
	want := []string{`unknown configuration key "ssh-host"`}
	if len(warns) != len(want) || warns[0] != want[0] {
		t.Fatalf("warnings=%v", warns)
	}
}

func TestEnvOverridesYAMLWithUnusedKeyWarn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	// Remove SSH flag bindings so ssh_host env var is unbound and warns.
	b.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("freeze-timeout: 1s\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_FREEZE_TIMEOUT", "2s")
	t.Setenv("LVMSYNC_SSH_HOST", "example")
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, warns, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.FreezeTimeout != 2*time.Second {
		t.Fatalf("FreezeTimeout=%v want %v", cfg.FreezeTimeout, 2*time.Second)
	}
	want := []string{`unknown configuration key "ssh-host"`}
	if len(warns) != len(want) || warns[0] != want[0] {
		t.Fatalf("warnings=%v", warns)
	}
}
