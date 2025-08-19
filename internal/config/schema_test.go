package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestSchemaValidationInvalidType(t *testing.T) {
	defaults, _ := DefaultConfig()
	b := NewBuilder(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgPath, []byte("parallel: 'nope'\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, _, _, err := b.Build(fs, []string{"--config", cfgPath})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestSchemaValidationValid(t *testing.T) {
	defaults, _ := DefaultConfig()
	b := NewBuilder(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgPath, []byte("parallel: 5\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, _, _, err := b.Build(fs, []string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Parallel != 5 {
		t.Fatalf("expected parallel 5, got %d", cfg.Parallel)
	}
}

func TestEnvDocUpToDate(t *testing.T) {
	defaults, _ := DefaultConfig()
	doc := EnvDoc(NewFlagSets(defaults))
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "env.md"))
	if err != nil {
		t.Fatalf("read env.md: %v", err)
	}
	if doc != string(data) {
		t.Fatalf("docs/env.md is out of date; run go run ./cmd/configdoc")
	}
}
