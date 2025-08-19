package compressiondetect_test

import (
	"testing"

	"github.com/spf13/pflag"

	"lvmsync_go/internal/config"
)

func TestCompressThresholdFlagOverridesDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := config.NewBuilder(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{})
	if err != nil {
		t.Fatalf("build default: %v", err)
	}
	if cfg.CompressThreshold != defaults.CompressThreshold {
		t.Fatalf("CompressThreshold=%v want %v", cfg.CompressThreshold, defaults.CompressThreshold)
	}

	b = config.NewBuilder(defaults)
	fs = pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err = b.Build(fs, []string{"--compress-threshold", "0.85"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.CompressThreshold != 0.85 {
		t.Fatalf("CompressThreshold=%v want %v", cfg.CompressThreshold, 0.85)
	}
}
