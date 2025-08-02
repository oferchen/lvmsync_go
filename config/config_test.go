package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestParseBytesOrFallback(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", "1KB")
		cb := &ConfigBuilder{v: v}
		if got := cb.parseBytesOrFallback("block_size", "4KB"); got != 1024 {
			t.Fatalf("expected 1024, got %d", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		v := viper.New()
		cb := &ConfigBuilder{v: v}
		if got := cb.parseBytesOrFallback("block_size", "2KB"); got != 2048 {
			t.Fatalf("expected 2048, got %d", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", "notbytes")
		cb := &ConfigBuilder{v: v}
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic")
			}
		}()
		cb.parseBytesOrFallback("block_size", "4KB")
	})
}

func TestConfigValidate(t *testing.T) {
	t.Run("vgdisplay success", func(t *testing.T) {
		dir := t.TempDir()
		script := filepath.Join(dir, "vgdisplay")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write script: %v", err)
		}
		t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
		cfg := &Config{VolumeGroup: "vg0"}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("vgdisplay failure", func(t *testing.T) {
		dir := t.TempDir()
		script := filepath.Join(dir, "vgdisplay")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatalf("write script: %v", err)
		}
		t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
		cfg := &Config{VolumeGroup: "vg0"}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
}
