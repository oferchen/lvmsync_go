package config

import (
	"testing"

	"github.com/spf13/viper"
)

// TestBuilderBuildSuccess verifies that Build returns a config using defaults
// when no explicit settings are provided.
func TestBuilderBuildSuccess(t *testing.T) {
	v := viper.New()
	v.Set("allow-insecure", true)
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	b := &builder{v: v, defaults: defaults}
	cfg, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cfg.Transport == "" {
		t.Fatalf("expected Transport to be set")
	}
}

// TestBuilderBuildInvalidThreshold verifies that Build fails when an invalid
// compression threshold is specified.
func TestBuilderBuildInvalidThreshold(t *testing.T) {
	v := viper.New()
	v.Set("allow-insecure", true)
	v.Set("compress-threshold", 2.0)
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	b := &builder{v: v, defaults: defaults}
	if _, err := b.Build(); err == nil {
		t.Fatalf("expected error for invalid compression threshold")
	}
}

// TestBuilderBuildInvalidZstdLevel verifies that an out-of-range zstd level
// causes Build to return an error.
func TestBuilderBuildInvalidZstdLevel(t *testing.T) {
	v := viper.New()
	v.Set("allow-insecure", true)
	v.Set("compress", "zstd")
	v.Set("zstd-level", 6)
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	b := &builder{v: v, defaults: defaults}
	if _, err := b.Build(); err == nil {
		t.Fatalf("expected error for invalid zstd level")
	}
}

// TestBuilderBuildInvalidLZ4Level verifies that an unsupported lz4 level
// causes Build to return an error.
func TestBuilderBuildInvalidLZ4Level(t *testing.T) {
	v := viper.New()
	v.Set("allow-insecure", true)
	v.Set("compress", "lz4")
	v.Set("lz4-level", "fastest")
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	b := &builder{v: v, defaults: defaults}
	if _, err := b.Build(); err == nil {
		t.Fatalf("expected error for invalid lz4 level")
	}
}
