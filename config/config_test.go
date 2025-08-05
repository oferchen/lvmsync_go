package config

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/pierrec/lz4/v4"
	"github.com/spf13/viper"
	compressiondetect "lvmsync_go/internal/compressiondetect"
	"lvmsync_go/lvm"
)

type fakeBackend struct {
	free uint64
	err  error
	vgs  []lvm.VolumeGroup
}

func (f *fakeBackend) CreateSnapshot(context.Context, string, string, string) error { return nil }
func (f *fakeBackend) RemoveSnapshot(context.Context, string) error                 { return nil }
func (f *fakeBackend) GetSnapshotUsage(context.Context, string) (float64, error)    { return 0, nil }
func (f *fakeBackend) GetVolumeGroupFreeSpace(context.Context, string) (uint64, error) {
	return f.free, f.err
}
func (f *fakeBackend) ListVolumeGroups(_ context.Context, candidates []string) ([]lvm.VolumeGroup, error) {
	if len(candidates) == 0 {
		return f.vgs, nil
	}
	res := []lvm.VolumeGroup{}
	for _, name := range candidates {
		for _, vg := range f.vgs {
			if vg.Name == name {
				res = append(res, vg)
				break
			}
		}
	}
	return res, nil
}

func TestDefaultConfigCompress(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Compress != "auto" {
		t.Fatalf("expected default Compress to be 'auto', got %q", cfg.Compress)
	}
}

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
	t.Run("success", func(t *testing.T) {
		fb := &fakeBackend{free: 100}
		restore := lvm.SetBackend(fb)
		defer restore()
		prev := lvm.GetEscalationCommand()
		lvm.SetEscalationCommand("sudo")
		defer lvm.SetEscalationCommand(prev)
		cfg := &Config{VolumeGroup: "vg0", LVMEscalation: "sudo"}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		fb := &fakeBackend{err: fmt.Errorf("command error")}
		restore := lvm.SetBackend(fb)
		defer restore()
		prev := lvm.GetEscalationCommand()
		lvm.SetEscalationCommand("sudo")
		defer lvm.SetEscalationCommand(prev)
		cfg := &Config{VolumeGroup: "vg0", LVMEscalation: "sudo"}
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

func TestCompressLevelValidation(t *testing.T) {
	t.Run("zstdValid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "zstd")
		v.Set("compress_level", 3)
		cb := &ConfigBuilder{v: v, defaults: DefaultConfig()}
		if _, err := cb.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("zstdInvalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "zstd")
		v.Set("compress_level", 100)
		cb := &ConfigBuilder{v: v, defaults: DefaultConfig()}
		if _, err := cb.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("autoValid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "auto")
		if compressiondetect.DetectOptimalCompression() == "zstd" {
			v.Set("compress_level", 3)
		} else {
			v.Set("compress_level", int(lz4.Level3))
		}
		cb := &ConfigBuilder{v: v, defaults: DefaultConfig()}
		if _, err := cb.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("autoInvalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "auto")
		if compressiondetect.DetectOptimalCompression() == "zstd" {
			v.Set("compress_level", 100)
		} else {
			v.Set("compress_level", 3)
		}
		cb := &ConfigBuilder{v: v, defaults: DefaultConfig()}
		if _, err := cb.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("lz4Valid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "lz4")
		v.Set("compress_level", int(lz4.Level3))
		cb := &ConfigBuilder{v: v, defaults: DefaultConfig()}
		if _, err := cb.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("lz4Invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "lz4")
		v.Set("compress_level", 3)
		cb := &ConfigBuilder{v: v, defaults: DefaultConfig()}
		if _, err := cb.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestCompressConcurrency(t *testing.T) {
	t.Run("defaultPositive", func(t *testing.T) {
		v := viper.New()
		cb := &ConfigBuilder{v: v, defaults: DefaultConfig()}
		cfg, err := cb.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CompressConcurrency <= 0 {
			t.Fatalf("expected positive concurrency, got %d", cfg.CompressConcurrency)
		}
	})

	t.Run("override", func(t *testing.T) {
		v := viper.New()
		v.Set("compress_concurrency", 8)
		cb := &ConfigBuilder{v: v, defaults: DefaultConfig()}
		cfg, err := cb.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CompressConcurrency != 8 {
			t.Fatalf("expected 8, got %d", cfg.CompressConcurrency)
		}
	})
}
