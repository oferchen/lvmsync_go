package config

import (
	"testing"
)

func TestSSHHostKeyPathCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "ssh_host_key_path: config.key\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--ssh_host_key_path", "cli.key"})
	t.Setenv("LVMSYNC_SSH_HOST_KEY_PATH", "env.key")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}
	v, _, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.SSHHostKeyPath != "cli.key" {
		t.Fatalf("expected ssh_host_key_path cli.key, got %s", conf.SSHHostKeyPath)
	}
}

func TestSSHHostKeyPathEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "ssh_host_key_path: config.key\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_SSH_HOST_KEY_PATH", "env.key")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}
	v, _, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.SSHHostKeyPath != "env.key" {
		t.Fatalf("expected ssh_host_key_path env.key, got %s", conf.SSHHostKeyPath)
	}
}
