package config

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func baseConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("lookpath true: %v", err)
	}
	cfg.FSFreezeCommand = truePath
	cfg.FSThawCommand = truePath
	return cfg
}

func TestValidateFSCommandsNULByte(t *testing.T) {
	cfg := baseConfig(t)
	cfg.FSFreezeCommand += "\x00"
	err := cfg.ValidateWith(func() int { return 0 })
	if err == nil || !strings.Contains(err.Error(), "NUL byte") {
		t.Fatalf("expected NUL byte error, got %v", err)
	}
}

func TestValidateFSCommandsInvalidChars(t *testing.T) {
	cfg := baseConfig(t)
	cfg.FSFreezeCommand = filepath.Join(filepath.Dir(cfg.FSFreezeCommand), "true!")
	err := cfg.ValidateWith(func() int { return 0 })
	if err == nil || !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("expected invalid character error, got %v", err)
	}
}

func TestValidateFSCommandsMissingBinary(t *testing.T) {
	cfg := baseConfig(t)
	cfg.FSThawCommand = filepath.Join(filepath.Dir(cfg.FSThawCommand), "does-not-exist")
	err := cfg.ValidateWith(func() int { return 0 })
	if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("expected missing binary error, got %v", err)
	}
}

func TestValidateFSCommandsMismatchedPair(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("lookpath true: %v", err)
	}
	cases := []struct {
		name   string
		freeze string
		thaw   string
	}{
		{"missing thaw", truePath, ""},
		{"missing freeze", "", truePath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseConfig(t)
			cfg.FSFreezeCommand = tc.freeze
			cfg.FSThawCommand = tc.thaw
			err := cfg.ValidateWith(func() int { return 0 })
			if err == nil || !strings.Contains(err.Error(), "must both be set") {
				t.Fatalf("expected mismatch error, got %v", err)
			}
		})
	}
}
