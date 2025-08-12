package config

import (
	"testing"

	"github.com/spf13/viper"
)

// TestBuilderBuildSuccess verifies that Build returns a config using defaults
// when no explicit settings are provided.
func TestBuilderBuildSuccess(t *testing.T) {
	v := viper.New()
	v.Set("allow_insecure", true)
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	b := &Builder{v: v, defaults: defaults}
	cfg, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cfg.Transport == "" {
		t.Fatalf("expected Transport to be set")
	}
}

// TestBuilderBuildUnsupportedCompression verifies that Build fails when an
// unsupported compression algorithm is specified.
func TestBuilderBuildUnsupportedCompression(t *testing.T) {
	v := viper.New()
	v.Set("allow_insecure", true)
	v.Set("compress_threshold", 2.0)
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	b := &Builder{v: v, defaults: defaults}
	if _, err := b.Build(); err == nil {
		t.Fatalf("expected error for invalid compression threshold")
	}
}
