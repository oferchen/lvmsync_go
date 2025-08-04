package config

import (
	"fmt"
	"math"
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
		got, err := cb.parseBytesOrFallback("block_size", "4KB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1000 {
			t.Fatalf("expected 1000, got %d", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		v := viper.New()
		cb := &ConfigBuilder{v: v}
		got, err := cb.parseBytesOrFallback("block_size", "2KB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 2000 {
			t.Fatalf("expected 2000, got %d", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", "notbytes")
		cb := &ConfigBuilder{v: v}
		if _, err := cb.parseBytesOrFallback("block_size", "4KB"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("nearMaxInt", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", fmt.Sprintf("%d", uint64(math.MaxInt-1023)))
		cb := &ConfigBuilder{v: v}
		got, err := cb.parseBytesOrFallback("block_size", "4KB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != math.MaxInt-1023 {
			t.Fatalf("expected %d, got %d", math.MaxInt-1023, got)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", fmt.Sprintf("%d", uint64(math.MaxInt)+1))
		cb := &ConfigBuilder{v: v}
		if _, err := cb.parseBytesOrFallback("block_size", "4KB"); err == nil {
			t.Fatalf("expected error")
		}
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
		cfg := DefaultConfig()
		cfg.VolumeGroup = "vg0"
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

func TestGetBlockSizeRaw(t *testing.T) {
	t.Run("fromConfig", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", "16KB")
		cb := &ConfigBuilder{v: v, defaults: &Config{BlockSizeRaw: "4KB"}}
		if got := cb.getBlockSizeRaw(); got != "16KB" {
			t.Fatalf("expected 16KB, got %s", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		v := viper.New()
		cb := &ConfigBuilder{v: v, defaults: &Config{BlockSizeRaw: "4KB"}}
		if got := cb.getBlockSizeRaw(); got != "4KB" {
			t.Fatalf("expected 4KB, got %s", got)
		}
	})
}
