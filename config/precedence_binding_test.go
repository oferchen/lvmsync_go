package config

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func TestCLIFlagsOverrideEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "parallel: 1\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--parallel", "3"})
	t.Setenv("LVMSYNC_PARALLEL", "2")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

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
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_PARALLEL", "2")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

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
	rootFS, parseArgs := newFlagSet(args)

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(parseArgs); err != nil {
		t.Fatalf("parse: %v", err)
	}

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
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--ssh_user", "cli"})
	t.Setenv("LVMSYNC_SSH_USER", "env")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

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
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_SSH_USER", "env")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

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
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--ssh_host", "cli"})
	t.Setenv("LVMSYNC_SSH_HOST", "env")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

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
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_SSH_HOST", "env")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

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

func TestTransportCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "transport: tcp+tls\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--transport", "ssh"})
	t.Setenv("LVMSYNC_TRANSPORT_TRANSPORT", "tcp+tls")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.Transport != "ssh" {
		t.Fatalf("expected transport ssh, got %s", conf.Transport)
	}
}

func TestTransportEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "transport: tcp+tls\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_TRANSPORT_TRANSPORT", "ssh")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.Transport != "ssh" {
		t.Fatalf("expected transport ssh, got %s", conf.Transport)
	}
}

func TestConcurrencyCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "concurrency: 1\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--concurrency", "3"})
	t.Setenv("LVMSYNC_TRANSPORT_CONCURRENCY", "2")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.Concurrency != 3 {
		t.Fatalf("expected concurrency 3, got %d", conf.Concurrency)
	}
}

func TestConcurrencyEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "concurrency: 1\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_TRANSPORT_CONCURRENCY", "2")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.Concurrency != 2 {
		t.Fatalf("expected concurrency 2, got %d", conf.Concurrency)
	}
}

func TestTCPPortCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "tcp_port: 1111\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--tcp_port", "3333"})
	t.Setenv("LVMSYNC_TRANSPORT_TCP_PORT", "2222")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.TCPPort != 3333 {
		t.Fatalf("expected tcp_port 3333, got %d", conf.TCPPort)
	}
}

func TestTCPPortEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "tcp_port: 1111\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_TRANSPORT_TCP_PORT", "2222")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.TCPPort != 2222 {
		t.Fatalf("expected tcp_port 2222, got %d", conf.TCPPort)
	}
}

func TestDedupStrategyCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "dedup_strategy: checksum\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--dedup_strategy", "bloom"})
	t.Setenv("LVMSYNC_DEDUP_STRATEGY", "rolling_hash")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.DedupStrategy != "bloom" {
		t.Fatalf("expected dedup_strategy bloom, got %s", conf.DedupStrategy)
	}
}

func TestDedupStrategyEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "dedup_strategy: checksum\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_DEDUP_STRATEGY", "bloom")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.DedupStrategy != "bloom" {
		t.Fatalf("expected dedup_strategy bloom, got %s", conf.DedupStrategy)
	}
}

func TestCDCMinCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "cdc_min: 512\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--cdc_min", "2048"})
	t.Setenv("LVMSYNC_DEDUP_CDC_MIN", "1024")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.CDCMin != 2048 {
		t.Fatalf("expected cdc_min 2048, got %d", conf.CDCMin)
	}
}

func TestCDCMinEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "cdc_min: 512\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_DEDUP_CDC_MIN", "1024")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.CDCMin != 1024 {
		t.Fatalf("expected cdc_min 1024, got %d", conf.CDCMin)
	}
}

func TestGRPCPortCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "grpc_port: 1111\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--grpc_port", "3333"})
	t.Setenv("LVMSYNC_GRPC_PORT", "2222")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.GRPCPort != 3333 {
		t.Fatalf("expected grpc_port 3333, got %d", conf.GRPCPort)
	}
}

func TestGRPCPortEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "grpc_port: 1111\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_GRPC_PORT", "2222")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.GRPCPort != 2222 {
		t.Fatalf("expected grpc_port 2222, got %d", conf.GRPCPort)
	}
}

// CLI flag should win over LVMSYNC_MANIFEST_PATH and manifest_path in YAML.
func TestManifestPathCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_path: cfg.manifest\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--manifest_path", "cli.manifest"})
	t.Setenv("LVMSYNC_MANIFEST_PATH", "env.manifest")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.ManifestPath != "cli.manifest" {
		t.Fatalf("expected manifest_path cli.manifest, got %s", conf.ManifestPath)
	}
}

// LVMSYNC_MANIFEST_PATH should override manifest_path from YAML when no flag is set.
func TestManifestPathEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_path: cfg.manifest\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_MANIFEST_PATH", "env.manifest")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.ManifestPath != "env.manifest" {
		t.Fatalf("expected manifest_path env.manifest, got %s", conf.ManifestPath)
	}
}

func TestManifestProgressIntervalCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_progress_interval: 1s\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--manifest_progress_interval", "3s"})
	t.Setenv("LVMSYNC_MANIFEST_PROGRESS_INTERVAL", "2s")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.ManifestProgressInterval != 3*time.Second {
		t.Fatalf("expected manifest_progress_interval 3s, got %v", conf.ManifestProgressInterval)
	}
}

func TestManifestProgressIntervalEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_progress_interval: 1s\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_MANIFEST_PROGRESS_INTERVAL", "2s")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.ManifestProgressInterval != 2*time.Second {
		t.Fatalf("expected manifest_progress_interval 2s, got %v", conf.ManifestProgressInterval)
	}
}

// CLI flag takes precedence over LVMSYNC_MANIFEST_TIMEOUT and YAML.
func TestManifestTimeoutCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_timeout: 1s\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--manifest_timeout", "3s"})
	t.Setenv("LVMSYNC_MANIFEST_TIMEOUT", "2s")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.ManifestTimeout != 3*time.Second {
		t.Fatalf("expected manifest_timeout 3s, got %v", conf.ManifestTimeout)
	}
}

// LVMSYNC_MANIFEST_TIMEOUT should override YAML when no CLI flag is present.
func TestManifestTimeoutEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_timeout: 1s\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_MANIFEST_TIMEOUT", "2s")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.ManifestTimeout != 2*time.Second {
		t.Fatalf("expected manifest_timeout 2s, got %v", conf.ManifestTimeout)
	}
}

// CLI flag should override environment variable and YAML for manifest_allow_mounted.
func TestManifestAllowMountedCLIOverridesEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_allow_mounted: true\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--manifest_allow_mounted=false"})
	t.Setenv("LVMSYNC_MANIFEST_ALLOW_MOUNTED", "true")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.ManifestAllowMounted {
		t.Fatalf("expected manifest_allow_mounted false, got %v", conf.ManifestAllowMounted)
	}
}

// LVMSYNC_MANIFEST_ALLOW_MOUNTED should override YAML when the CLI flag is absent.
func TestManifestAllowMountedEnvOverridesConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_allow_mounted: true\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath})
	t.Setenv("LVMSYNC_MANIFEST_ALLOW_MOUNTED", "false")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	builder := &Builder{v: v, defaults: defaults}
	conf, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.ManifestAllowMounted {
		t.Fatalf("expected manifest_allow_mounted false, got %v", conf.ManifestAllowMounted)
	}
}

// Subsetting FlagSets should still bind remaining flags and omit removed ones.
func TestSubsetFlagSetsBinding(t *testing.T) {
	cfgPath := writeTempConfig(t, "manifest_path: cfg.manifest\n")
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--manifest_path", "cli.manifest"})

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	fs.SSH = pflag.NewFlagSet("SSH Options", pflag.ExitOnError)
	fs.Remote = pflag.NewFlagSet("Remote Options", pflag.ExitOnError)
	fs.Dedup = pflag.NewFlagSet("Deduplication Options", pflag.ExitOnError)
	fs.Compression = pflag.NewFlagSet("Compression Options", pflag.ExitOnError)
	fs.LVM = pflag.NewFlagSet("LVM Options", pflag.ExitOnError)
	fs.GRPC = pflag.NewFlagSet("gRPC Options", pflag.ExitOnError)
	fs.Transport = pflag.NewFlagSet("Transport Options", pflag.ExitOnError)
	registerFlags(fs, rootFS)
	if err := rootFS.Parse(args); err != nil {
		t.Fatalf("parse: %v", err)
	}

	v, err := buildViper(fs)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	if got := v.GetString("manifest_path"); got != "cli.manifest" {
		t.Fatalf("manifest_path got %q want %q", got, "cli.manifest")
	}
	if rootFS.Lookup("ssh_user") != nil {
		t.Fatalf("unexpected ssh_user flag present")
	}
}
