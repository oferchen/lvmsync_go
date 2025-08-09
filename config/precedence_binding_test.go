package config

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestCLIFlagsOverrideEnvAndConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, "parallel: 1\n")
	resetFlags([]string{"--config", cfgPath, "--parallel", "3"})
	t.Setenv("LVMSYNC.PARALLEL", "2")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	registerFlags(defaults)
	pflag.Parse()

	v, err := buildViper()
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
	t.Setenv("LVMSYNC.PARALLEL", "2")

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	registerFlags(defaults)
	pflag.Parse()

	v, err := buildViper()
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
	}
	resetFlags(args)

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	registerFlags(defaults)
	pflag.Parse()

	v, err := buildViper()
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
}
