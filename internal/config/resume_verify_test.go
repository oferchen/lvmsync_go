package config

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestResumeVerifyFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LVMSYNC_RESUME", "statefile")
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	b := NewBuilder(defaults)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg, _, _, err := b.Build(fs, []string{"--resume=verify"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !cfg.ResumeVerify {
		t.Fatalf("expected ResumeVerify true")
	}
	if cfg.ResumeState != "statefile" {
		t.Fatalf("ResumeState=%q want %q", cfg.ResumeState, "statefile")
	}
}
