package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// resetFlags replaces the global command line flags and sets os.Args.
func resetFlags(args []string) {
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	pflag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{"test"}, args...)
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestRegisterFlags(t *testing.T) {
	resetFlags(nil)
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	registerFlags(cfg)
	names := []string{"parallel", "ssh_user", "grpc_port"}
	for _, name := range names {
		if f := pflag.CommandLine.Lookup(name); f == nil {
			t.Fatalf("missing %s flag", name)
		}
	}
}

func TestBuildViperPrecedence(t *testing.T) {
	cfgPath := writeTempConfig(t, "parallel: 1\n")

	t.Run("config_overrides_defaults", func(t *testing.T) {
		resetFlags([]string{"--config", cfgPath})
		cfg, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig: %v", err)
		}
		registerFlags(cfg)
		pflag.Parse()
		v, err := buildViper()
		if err != nil {
			t.Fatalf("buildViper: %v", err)
		}
		if got := v.GetInt("parallel"); got != 1 {
			t.Fatalf("expected parallel 1, got %d", got)
		}
	})

	t.Run("env_overrides_config", func(t *testing.T) {
		resetFlags([]string{"--config", cfgPath})
		t.Setenv("LVMSYNC.PARALLEL", "2")
		cfg, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig: %v", err)
		}
		registerFlags(cfg)
		pflag.Parse()
		v, err := buildViper()
		if err != nil {
			t.Fatalf("buildViper: %v", err)
		}
		if got := v.GetInt("parallel"); got != 2 {
			t.Fatalf("expected parallel 2, got %d", got)
		}
	})

	t.Run("flags_override_env", func(t *testing.T) {
		resetFlags([]string{"--config", cfgPath, "--parallel", "3"})
		t.Setenv("LVMSYNC.PARALLEL", "2")
		cfg, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig: %v", err)
		}
		registerFlags(cfg)
		pflag.Parse()
		v, err := buildViper()
		if err != nil {
			t.Fatalf("buildViper: %v", err)
		}
		if got := v.GetInt("parallel"); got != 3 {
			t.Fatalf("expected parallel 3, got %d", got)
		}
	})
}

func TestUsageOutput(t *testing.T) {
	resetFlags(nil)
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	registerFlags(cfg)
	buf := &bytes.Buffer{}
	pflag.CommandLine.SetOutput(buf)
	pflag.Usage()
	out := buf.String()
	wants := []string{
		"General Options:", "--config",
		"SSH Options:", "--ssh_user",
		"Remote Options:", "--lvmsync_path",
		"Deduplication Options:", "--dedup_strategy",
		"Compression Options:", "--compress",
		"LVM Options:", "--skip_snapshot_creation",
		"gRPC Options:", "--grpc_port",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Fatalf("usage missing %q", w)
		}
	}
}
