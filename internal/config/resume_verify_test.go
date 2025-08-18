package config

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestResumeFlagPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--resume=statefile"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cfg.ResumeVerify {
		t.Fatalf("ResumeVerify=%v want false", cfg.ResumeVerify)
	}
	if cfg.ResumeState != "statefile" {
		t.Fatalf("ResumeState=%q want %q", cfg.ResumeState, "statefile")
	}
}

func TestResumeFlagVerify(t *testing.T) {
	t.Skip("resume verify precedence not verified")
}
