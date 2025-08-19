package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestSchemaValidationUnknownType(t *testing.T) {
	defaults, _ := DefaultConfig()
	b := NewBuilder(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	// write temp config with wrong type for parallel (string instead of int)
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

func TestEnvDocUpToDate(t *testing.T) {
	defaults, _ := DefaultConfig()
	doc := EnvDocHeader + EnvDoc(NewFlagSets(defaults))
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "config_env.md"))
	if err != nil {
		t.Fatalf("read config_env.md: %v", err)
	}
	if doc != string(data) {
		t.Fatalf("docs/config_env.md is out of date; run go generate ./internal/config")
	}
}
