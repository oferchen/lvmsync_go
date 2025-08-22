package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestUnknownKeysProduceError(t *testing.T) {
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
	_, _, _, err = b.Build(fs, []string{"--config", cfgFile})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestAllowInsecureRequiresFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("allow_insecure: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, _, _, err = b.Build(fs, []string{"--config", cfgFile})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestAllowInsecureFlagWarns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, warns, err := b.Build(fs, []string{"--allow-insecure"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !cfg.AllowInsecure {
		t.Fatalf("expected AllowInsecure to be true")
	}
	want := "allow_insecure enabled; security checks disabled"
	if len(warns) != 1 || warns[0] != want {
		t.Fatalf("warnings=%v", warns)
	}
}

func TestUnboundConfigKeyWarns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	// Remove SSH flag bindings so ssh_host becomes unbound.
	b.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("ssh_host: example\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, _, warns, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := `unknown configuration key "ssh-host"`
	if len(warns) != 1 || warns[0] != want {
		t.Fatalf("warnings=%v", warns)
	}
}

func TestUnboundEnvKeyWarns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LVMSYNC_SSH_HOST", "example")
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	b.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, _, warns, err := b.Build(fs, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := `unknown configuration key "ssh-host"`
	if len(warns) != 1 || warns[0] != want {
		t.Fatalf("warnings=%v", warns)
	}
}

func TestWarningsSorted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	// Remove SSH flag bindings so multiple keys become unbound.
	b.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("ssh_host: example\nssh_user: root\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, _, warns, err := b.Build(fs, []string{"--config", cfgFile})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := []string{
		`unknown configuration key "ssh-host"`,
		`unknown configuration key "ssh-user"`,
	}
	if len(warns) != len(want) || warns[0] != want[0] || warns[1] != want[1] {
		t.Fatalf("warnings=%v", warns)
	}
}

func TestStrictConfigYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	b.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("ssh_host: example\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, _, _, err = b.Build(fs, []string{"--config", cfgFile, "--strict-config"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestStrictConfigEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LVMSYNC_SSH_HOST", "example")
	t.Setenv("LVMSYNC_STRICT_CONFIG", "1")
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	b.FlagSets.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, _, _, err = b.Build(fs, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestStrictConfigAllowInsecureFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	_, _, _, err = b.Build(fs, []string{"--allow-insecure", "--strict-config"})
	if err == nil {
		t.Fatalf("expected error")
	}
}
