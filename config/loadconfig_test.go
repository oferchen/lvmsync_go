package config

import (
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

func TestLoadConfigPrecedence(t *testing.T) {
	cfgPath := writeTempConfig(t, "parallel: 1\n")

	t.Run("config_overrides_defaults", func(t *testing.T) {
		resetFlags([]string{"--config", cfgPath})
		c, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if c.Parallel != 1 {
			t.Fatalf("expected parallel 1, got %d", c.Parallel)
		}
	})

	t.Run("env_overrides_config", func(t *testing.T) {
		resetFlags([]string{"--config", cfgPath})
		t.Setenv("LVMSYNC.PARALLEL", "2")
		c, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if c.Parallel != 2 {
			t.Fatalf("expected parallel 2, got %d", c.Parallel)
		}
	})

	t.Run("flags_override_env", func(t *testing.T) {
		resetFlags([]string{"--config", cfgPath, "--parallel", "3"})
		t.Setenv("LVMSYNC.PARALLEL", "2")
		c, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if c.Parallel != 3 {
			t.Fatalf("expected parallel 3, got %d", c.Parallel)
		}
	})
}

func TestUsageIncludesFlagGroups(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	initFlagSets(cfg)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	printUsage()
	w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(out)
	headings := []string{
		"General Options:",
		"SSH Options:",
		"Remote Options:",
		"Deduplication Options:",
		"Compression Options:",
		"LVM Options:",
		"gRPC Options:",
	}
	for _, h := range headings {
		if !strings.Contains(s, h) {
			t.Fatalf("usage missing %q", h)
		}
	}
}
