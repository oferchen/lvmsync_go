package main

import (
	"github.com/spf13/pflag"
	"lvmsync_go/config"
	"testing"
)

func TestApplyFlagTriggersApplyMode(t *testing.T) {
	// Setup configuration with apply file
	var err error
	cfg, err = config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.ApplyMode = "dumpfile"
	dest := "/dev/null"

	// Prepare flag set with destination argument
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	pflag.CommandLine = fs
	if err := fs.Parse([]string{dest}); err != nil {
		t.Fatalf("failed to parse args: %v", err)
	}

	called := false
	original := applyFunc
	applyFunc = func(c *config.Config, applyFile, destDevice string) error {
		called = true
		if applyFile != cfg.ApplyMode {
			t.Fatalf("expected applyFile %s, got %s", cfg.ApplyMode, applyFile)
		}
		if destDevice != dest {
			t.Fatalf("expected dest %s, got %s", dest, destDevice)
		}
		return nil
	}
	defer func() { applyFunc = original }()

	if err := runApplyMode(cfg.ApplyMode); err != nil {
		t.Fatalf("runApplyMode returned error: %v", err)
	}
	if !called {
		t.Fatalf("applyFunc was not called")
	}
}
