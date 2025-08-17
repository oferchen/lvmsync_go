package config

import (
	"testing"

	"github.com/spf13/viper"
)

// TestBuilderBuildSuccess verifies that Build returns a config using defaults
// when no explicit settings are provided.
func TestBuilderBuildSuccess(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	for _, k := range []string{"allow-insecure", "allow_insecure"} {
		t.Run(k, func(t *testing.T) {
			v := viper.New()
			v.Set(k, true)
			b := &builder{v: v, defaults: defaults}
			cfg, err := b.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if cfg.Transport == "" {
				t.Fatalf("expected Transport to be set")
			}
		})
	}
}

// TestBuilderBuildUnsupportedCompression verifies that Build fails when an
// unsupported compression algorithm is specified.
func TestBuilderBuildUnsupportedCompression(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	for _, k := range []string{"compress-threshold", "compress_threshold"} {
		t.Run(k, func(t *testing.T) {
			v := viper.New()
			v.Set("allow-insecure", true)
			v.Set(k, 2.0)
			b := &builder{v: v, defaults: defaults}
			if _, err := b.Build(); err == nil {
				t.Fatalf("expected error for invalid compression threshold")
			}
		})
	}
}
