package config

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

type timeoutBackend struct{}

func (timeoutBackend) CreateSnapshot(context.Context, string, string, string) error { return nil }
func (timeoutBackend) RemoveSnapshot(context.Context, string) error                 { return nil }
func (timeoutBackend) GetSnapshotUsage(context.Context, string) (float64, error)    { return 0, nil }
func (timeoutBackend) GetVolumeGroupFreeSpace(ctx context.Context, _ string) (uint64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(10 * time.Millisecond):
		return 0, nil
	}
}
func (timeoutBackend) ListVolumeGroups(context.Context, []string) ([]lvm.VolumeGroup, error) {
	return nil, nil
}

func TestDefaultConfigCompress(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	if cfg.Compress != Auto {
		t.Fatalf("expected default Compress to be %q, got %q", Auto, cfg.Compress)
	}
}

func TestDefaultConfigBlockSize(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	if cfg.BlockSize != 0 || cfg.BlockSizeRaw != Auto {
		t.Fatalf("expected default block size %s, got %d (%s)", Auto, cfg.BlockSize, cfg.BlockSizeRaw)
	}
}

func TestDefaultConfigStrictHostKeyCheck(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	if !cfg.StrictHostKeyCheck {
		t.Fatalf("expected StrictHostKeyCheck to default to true")
	}
}

func TestDefaultConfigHeartbeat(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	if cfg.HeartbeatInterval != 30*time.Second || cfg.HeartbeatSendTimeout != 5*time.Second {
		t.Fatalf("unexpected heartbeat defaults %v %v", cfg.HeartbeatInterval, cfg.HeartbeatSendTimeout)
	}
}

func TestHumanBlockSize(t *testing.T) {
	c := &Config{BlockSize: 0}
	if c.HumanBlockSize() != Auto {
		t.Fatalf("expected %s, got %s", Auto, c.HumanBlockSize())
	}
}

//nolint:revive // complex test cases handled in subtests
func TestParseBytesOrFallback(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", "1KB")
		b := &Builder{v: v}
		got, err := b.parseBytesOrFallback("block_size", "4KB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1000 {
			t.Fatalf("expected 1000, got %d", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		v := viper.New()
		b := &Builder{v: v}
		got, err := b.parseBytesOrFallback("block_size", "2KB")
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
		b := &Builder{v: v}
		if _, err := b.parseBytesOrFallback("block_size", "4KB"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("nearMaxInt", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", fmt.Sprintf("%d", uint64(math.MaxInt-1023)))
		b := &Builder{v: v}
		got, err := b.parseBytesOrFallback("block_size", "4KB")
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
		b := &Builder{v: v}
		if _, err := b.parseBytesOrFallback("block_size", "4KB"); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestBuilderApplyDefaults(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	v := viper.New()
	b := &Builder{v: v, defaults: defaults}
	var conf Config
	if err := b.applyDefaults(&conf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conf.AllowInsecure != defaults.AllowInsecure {
		t.Fatalf("expected AllowInsecure %v", defaults.AllowInsecure)
	}
	if conf.GRPCPort != defaults.GRPCPort {
		t.Fatalf("expected GRPCPort %d, got %d", defaults.GRPCPort, conf.GRPCPort)
	}
	if conf.BlockSize != 0 || conf.BlockSizeRaw != Auto {
		t.Fatalf("expected %s block size, got %d/%s", Auto, conf.BlockSize, conf.BlockSizeRaw)
	}
	if conf.CompressConcurrency != runtime.GOMAXPROCS(0) {
		t.Fatalf("expected default compress concurrency, got %d", conf.CompressConcurrency)
	}
	if conf.Concurrency != 0 {
		t.Fatalf("expected default concurrency 0, got %d", conf.Concurrency)
	}
	if conf.SyncIntervalBytes != defaults.SyncIntervalBytes {
		t.Fatalf("expected default sync interval %d, got %d", defaults.SyncIntervalBytes, conf.SyncIntervalBytes)
	}

	t.Run("invalidBlockSize", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", "bad")
		b := &Builder{v: v, defaults: defaults}
		if err := b.applyDefaults(&conf); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestBuilderValidateCompression(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	b := &Builder{defaults: defaults}

	t.Run(Zstd+"Valid", func(t *testing.T) {
		conf := &Config{Compress: Zstd, ZstdLevel: 3, CompressThreshold: 0.9}
		if err := b.validateCompression(conf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run(Zstd+"Invalid", func(t *testing.T) {
		conf := &Config{Compress: Zstd, ZstdLevel: 6, CompressThreshold: 0.9}
		if err := b.validateCompression(conf); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidThresholdTooHigh", func(t *testing.T) {
		conf := &Config{Compress: Zstd, ZstdLevel: 3, CompressThreshold: 1.1}
		if err := b.validateCompression(conf); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidThresholdNonPositive", func(t *testing.T) {
		conf := &Config{Compress: Zstd, ZstdLevel: 3, CompressThreshold: 0}
		if err := b.validateCompression(conf); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run(Auto, func(t *testing.T) {
		conf := &Config{Compress: Auto, CompressThreshold: 0.9}
		if compressiondetect.DetectOptimalCompression() == Zstd {
			conf.ZstdLevel = 2
		} else {
			conf.LZ4Level = "fast"
		}
		if err := b.validateCompression(conf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestBuilderFinalizeConfig(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	b := &Builder{defaults: defaults}

	t.Run("validInsecure", func(t *testing.T) {
		conf := &Config{AllowInsecure: true, ChecksumAlgorithm: "sha256"}
		if err := b.finalizeConfig(conf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conf.ChecksumAlgorithm != "sha256" {
			t.Fatalf("expected checksum algorithm normalized")
		}
	})

	t.Run("invalidAlgorithm", func(t *testing.T) {
		conf := &Config{AllowInsecure: true, ChecksumAlgorithm: "md5"}
		if err := b.finalizeConfig(conf); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missingTLS", func(t *testing.T) {
		conf := &Config{AllowInsecure: false, ChecksumAlgorithm: "sha256", GRPCListen: ":1"}
		if err := b.finalizeConfig(conf); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("validTLS", func(t *testing.T) {
		dir := t.TempDir()
		cert := filepath.Join(dir, "cert.pem")
		key := filepath.Join(dir, "key.pem")
		if err := os.WriteFile(cert, []byte("cert"), 0o644); err != nil {
			t.Fatalf("write cert: %v", err)
		}
		if err := os.WriteFile(key, []byte("key"), 0o644); err != nil {
			t.Fatalf("write key: %v", err)
		}
		conf := &Config{AllowInsecure: false, TLSCert: cert, TLSKey: key, GRPCListen: ":1", ChecksumAlgorithm: "sha256"}
		if err := b.finalizeConfig(conf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestConfigValidate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fb := &fakeBackend{free: 100}
		restore := lvm.SetBackend(fb)
		defer restore()
		restorePriv := lvm.SetPrivilegeChecker(func() error { return nil })
		defer restorePriv()
		cfg := &Config{
			VolumeGroup:          "vg0",
			LVMEscalation:        "sudo",
			SSHKeepAliveInterval: time.Second,
			LVMTimeout:           time.Second,
			GRPCDialTimeout:      time.Second,
			HeartbeatInterval:    time.Second,
			HeartbeatSendTimeout: time.Second,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		fb := &fakeBackend{err: fmt.Errorf("command error")}
		restore := lvm.SetBackend(fb)
		defer restore()
		restorePriv := lvm.SetPrivilegeChecker(func() error { return nil })
		defer restorePriv()
		cfg := &Config{VolumeGroup: "vg0", LVMEscalation: "sudo", SSHKeepAliveInterval: time.Second, LVMTimeout: time.Second}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidKeepalive", func(t *testing.T) {
		cfg := &Config{SSHKeepAliveInterval: 0, GRPCDialTimeout: time.Second, HeartbeatInterval: time.Second, HeartbeatSendTimeout: time.Second}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidHeartbeatInterval", func(t *testing.T) {
		cfg := &Config{SSHKeepAliveInterval: time.Second, GRPCDialTimeout: time.Second, HeartbeatInterval: 0, HeartbeatSendTimeout: time.Second}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidHeartbeatSendTimeout", func(t *testing.T) {
		cfg := &Config{SSHKeepAliveInterval: time.Second, GRPCDialTimeout: time.Second, HeartbeatInterval: time.Second, HeartbeatSendTimeout: 0}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		restore := lvm.SetBackend(timeoutBackend{})
		defer restore()
		restorePriv := lvm.SetPrivilegeChecker(func() error { return nil })
		defer restorePriv()
		cfg := &Config{VolumeGroup: "vg0", LVMEscalation: "sudo", SSHKeepAliveInterval: time.Second, LVMTimeout: time.Millisecond}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected timeout error")
		}
	})
}

func TestApplyThroughputMode(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	v := viper.New()
	conf := &Config{Mode: "throughput"}
	b := &Builder{v: v, defaults: defaults}
	if err := b.applyDefaults(conf); err != nil {
		t.Fatalf("applyDefaults returned error: %v", err)
	}
	if conf.Transport != "quic,h2,tcp+tls" {
		t.Fatalf("transport order %s", conf.Transport)
	}
	if conf.Parallel != 8 {
		t.Fatalf("parallel %d", conf.Parallel)
	}
	if conf.Concurrency != 8 {
		t.Fatalf("concurrency %d", conf.Concurrency)
	}
	if conf.DedupMode != "hybrid" {
		t.Fatalf("dedup mode %s", conf.DedupMode)
	}
	if conf.BlockSize != 2*1024*1024 {
		t.Fatalf("block size %d", conf.BlockSize)
	}
	if conf.CDCMin != 256*1024 || conf.CDCAvg != 2*1024*1024 || conf.CDCMax != 8*1024*1024 {
		t.Fatalf("cdc ranges %d %d %d", conf.CDCMin, conf.CDCAvg, conf.CDCMax)
	}
	if conf.Compress != Auto {
		t.Fatalf("compress %s", conf.Compress)
	}
	if !conf.ODirect {
		t.Fatalf("expected odirect enabled")
	}
	if conf.SyncIntervalBytes != 1000000000 {
		t.Fatalf("sync interval bytes %d", conf.SyncIntervalBytes)
	}
	if conf.CheckpointInterval != 10*time.Second {
		t.Fatalf("checkpoint interval %v", conf.CheckpointInterval)
	}
	if conf.QUICCongestionControl != "bbr" {
		t.Fatalf("quic cc %s", conf.QUICCongestionControl)
	}
}

func TestValidateEscalationCommandPath(t *testing.T) {
	geteuid := func() int { return 1000 }

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	exe := filepath.Join(dir, "sudo")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("failed to create dummy executable: %v", err)
	}
	os.Setenv("PATH", dir)

	cfg := &Config{LVMEscalation: "sudo", SSHKeepAliveInterval: time.Second, GRPCDialTimeout: time.Second}
	if err := cfg.ValidateWith(geteuid); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	os.Setenv("PATH", "")
	if err := cfg.ValidateWith(geteuid); err == nil {
		t.Fatalf("expected error when command missing")
	}
}

func TestBuildBlockSize(t *testing.T) {
	t.Run(Auto, func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", Auto)
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		cfg, err := b.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BlockSize != 0 || cfg.BlockSizeRaw != Auto {
			t.Fatalf("expected %s block size, got %d (%s)", Auto, cfg.BlockSize, cfg.BlockSizeRaw)
		}
	})

	t.Run("numeric", func(t *testing.T) {
		v := viper.New()
		v.Set("block_size", "8KB")
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		cfg, err := b.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BlockSize != 8000 || cfg.BlockSizeRaw != "8KB" {
			t.Fatalf("expected 8000/8KB, got %d/%s", cfg.BlockSize, cfg.BlockSizeRaw)
		}
	})
}

//nolint:revive // complex validation scenarios
func TestCompressionLevelValidation(t *testing.T) {
	t.Run(Zstd+"Valid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", Zstd)
		v.Set("zstd_level", 3)
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run(Zstd+"Invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", Zstd)
		v.Set("zstd_level", 6)
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run(Auto+"Valid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", Auto)
		if compressiondetect.DetectOptimalCompression() == Zstd {
			v.Set("zstd_level", 2)
		} else {
			v.Set("lz4_level", "fast")
		}
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run(Auto+"Invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", Auto)
		if compressiondetect.DetectOptimalCompression() == Zstd {
			v.Set("zstd_level", 6)
		} else {
			v.Set("lz4_level", "bad")
		}
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("lz4Valid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "lz4")
		v.Set("lz4_level", "hc")
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("lz4Invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "lz4")
		v.Set("lz4_level", "slow")
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestCompressConcurrency(t *testing.T) {
	t.Run("defaultPositive", func(t *testing.T) {
		v := viper.New()
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		cfg, err := b.Build()
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
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &Builder{v: v, defaults: defaults}
		cfg, err := b.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CompressConcurrency != 8 {
			t.Fatalf("expected 8, got %d", cfg.CompressConcurrency)
		}
	})
}

func TestTLSFileValidation(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}

	t.Run("insecure", func(t *testing.T) {
		v := viper.New()
		v.Set("allow_insecure", true)
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missingFiles", func(t *testing.T) {
		v := viper.New()
		v.Set("allow_insecure", false)
		v.Set("grpc_listen", ":1")
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("validFiles", func(t *testing.T) {
		dir := t.TempDir()
		cert := filepath.Join(dir, "cert.pem")
		key := filepath.Join(dir, "key.pem")
		ca := filepath.Join(dir, "ca.pem")
		for _, f := range []string{cert, key, ca} {
			if err := os.WriteFile(f, []byte("dummy"), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}
		}
		v := viper.New()
		v.Set("allow_insecure", false)
		v.Set("grpc_listen", ":1")
		v.Set("tls_cert", cert)
		v.Set("tls_key", key)
		v.Set("ca_cert", ca)
		b := &Builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDefaultCDCTunables(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	if cfg.CDCMin != 4*1024 || cfg.CDCAvg != 64*1024 || cfg.CDCMax != 1*1024*1024 {
		t.Fatalf("unexpected CDC defaults %d/%d/%d", cfg.CDCMin, cfg.CDCAvg, cfg.CDCMax)
	}
	if cfg.BloomMBits != 0 {
		t.Fatalf("expected bloom mbits 0, got %d", cfg.BloomMBits)
	}
}

func TestLoadConfigPrecedence(t *testing.T) {
	cfgPath := writeTempConfig(t, "parallel: 1\n")
	resetFlags([]string{"--config", cfgPath, "--parallel", "3"})
	t.Setenv("LVMSYNC_PARALLEL", "2")
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	conf, err := LoadConfig(fs, defaults)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if conf.Parallel != 3 {
		t.Fatalf("expected parallel 3, got %d", conf.Parallel)
	}
}

func TestLoadConfigInvalidPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	resetFlags([]string{"--config", missing})

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	if _, err := LoadConfig(fs, defaults); err == nil || !strings.Contains(err.Error(), "error reading config file") {
		t.Fatalf("expected config file error, got %v", err)
	}
}
