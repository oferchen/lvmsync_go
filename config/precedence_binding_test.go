package config

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestCLIFlagsOverrideEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "parallel: 1\n")
	resetFlags([]string{"--config", cfgPath, "--parallel", "3"})
	t.Setenv("LVMSYNC_PARALLEL", "2")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs)
	pflag.Parse()

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.Parallel != 3 {
		t.Fatalf("expected parallel 3, got %d", conf.Parallel)
	}
}

func TestEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "parallel: 1\n")
	resetFlags([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_PARALLEL", "2")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs)
	pflag.Parse()

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.Parallel != 2 {
		t.Fatalf("expected parallel 2, got %d", conf.Parallel)
	}
}

func TestFlagSetsBindToViper(t *testing.T) {
	args := []string{
		"--parallel", "5",
		"--ssh_user", "alice",
		"--lvmsync_path", "/usr/bin/lvmsync",
		"--dedup_strategy", "checksum",
		"--compress", "lz4",
		"--skip_snapshot_creation=true",
		"--grpc_port", "9999",
		"--grpc_heartbeat_interval", "2s",
		"--grpc_heartbeat_send_timeout", "1s",
	}
	resetFlags(args)

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs)
	pflag.Parse()

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}

	if got := v.GetInt("parallel"); got != 5 {
		t.Fatalf("parallel got %d want 5", got)
	}
	if got := v.GetString("ssh_user"); got != "alice" {
		t.Fatalf("ssh_user got %q want %q", got, "alice")
	}
	if got := v.GetString("lvmsync_path"); got != "/usr/bin/lvmsync" {
		t.Fatalf("lvmsync_path got %q want %q", got, "/usr/bin/lvmsync")
	}
	if got := v.GetString("dedup_strategy"); got != "checksum" {
		t.Fatalf("dedup_strategy got %q want %q", got, "checksum")
	}
	if got := v.GetString("compress"); got != "lz4" {
		t.Fatalf("compress got %q want %q", got, "lz4")
	}
	if got := v.GetBool("skip_snapshot_creation"); !got {
		t.Fatalf("skip_snapshot_creation got %v want true", got)
	}
	if got := v.GetInt("grpc_port"); got != 9999 {
		t.Fatalf("grpc_port got %d want 9999", got)
	}
	if got := v.GetDuration("grpc_heartbeat_interval"); got != 2*time.Second {
		t.Fatalf("grpc_heartbeat_interval got %v want 2s", got)
	}
	if got := v.GetDuration("grpc_heartbeat_send_timeout"); got != time.Second {
		t.Fatalf("grpc_heartbeat_send_timeout got %v want 1s", got)
	}
}

func TestSSHUserCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "ssh_user: config\n")
	resetFlags([]string{"--config", cfgPath, "--ssh_user", "cli"})
	t.Setenv("LVMSYNC_SSH_USER", "env")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs)
	pflag.Parse()

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.SSHUser != "cli" {
		t.Fatalf("expected ssh_user cli, got %s", conf.SSHUser)
	}
}

func TestSSHUserEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "ssh_user: config\n")
	resetFlags([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_SSH_USER", "env")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs)
	pflag.Parse()

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.SSHUser != "env" {
		t.Fatalf("expected ssh_user env, got %s", conf.SSHUser)
	}
}

func TestSSHHostCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "ssh_host: config\n")
	resetFlags([]string{"--config", cfgPath, "--ssh_host", "cli"})
	t.Setenv("LVMSYNC_SSH_HOST", "env")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs)
	pflag.Parse()

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.SSHHost != "cli" {
		t.Fatalf("expected ssh_host cli, got %s", conf.SSHHost)
	}
}

func TestSSHHostEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "ssh_host: config\n")
	resetFlags([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_SSH_HOST", "env")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs)
	pflag.Parse()

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.SSHHost != "env" {
		t.Fatalf("expected ssh_host env, got %s", conf.SSHHost)
	}
}
