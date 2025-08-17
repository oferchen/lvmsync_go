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
	digest "lvmsync_go/internal/digest"
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

func TestBlockSizeBytes(t *testing.T) {
	cases := []struct {
		in   int
		want uint64
	}{
		{in: 0, want: 0},
		{in: 4096, want: 4096},
	}
	for _, tc := range cases {
		c := &Config{BlockSize: tc.in}
		if got := c.BlockSizeBytes(); got != tc.want {
			t.Fatalf("block size bytes for %d: %d", tc.in, got)
		}
	}
}

//nolint:revive // complex test cases handled in subtests
func TestParseBytesOrFallback(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := viper.New()
		v.Set("block-size", "1KB")
		b := &builder{v: v}
		got, err := b.parseBytesOrFallback("block-size", "4KB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1000 {
			t.Fatalf("expected 1000, got %d", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		v := viper.New()
		b := &builder{v: v}
		got, err := b.parseBytesOrFallback("block-size", "2KB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 2000 {
			t.Fatalf("expected 2000, got %d", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("block-size", "notbytes")
		b := &builder{v: v}
		if _, err := b.parseBytesOrFallback("block-size", "4KB"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("nearMaxInt", func(t *testing.T) {
		v := viper.New()
		v.Set("block-size", fmt.Sprintf("%d", uint64(math.MaxInt-1023)))
		b := &builder{v: v}
		got, err := b.parseBytesOrFallback("block-size", "4KB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != math.MaxInt-1023 {
			t.Fatalf("expected %d, got %d", math.MaxInt-1023, got)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		v := viper.New()
		v.Set("block-size", fmt.Sprintf("%d", uint64(math.MaxInt)+1))
		b := &builder{v: v}
		if _, err := b.parseBytesOrFallback("block-size", "4KB"); err == nil {
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
	b := &builder{v: v, defaults: defaults}
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
		v.Set("block-size", "bad")
		b := &builder{v: v, defaults: defaults}
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
	b := &builder{defaults: defaults}

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

	t.Run("thresholdNonPositive", func(t *testing.T) {
		conf := &Config{Compress: Zstd, ZstdLevel: 3, CompressThreshold: 0}
		if err := b.validateCompression(conf); err != nil {
			t.Fatalf("unexpected error: %v", err)
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
	b := &builder{defaults: defaults}

	t.Run("validInsecure", func(t *testing.T) {
		conf := &Config{AllowInsecure: true, ChecksumAlgorithm: "sha256"}
		if err := b.finalizeConfig(conf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conf.ChecksumAlgorithm != "sha256" {
			t.Fatalf("expected checksum algorithm normalized")
		}
	})

	t.Run("autoAlgorithm", func(t *testing.T) {
		conf := &Config{AllowInsecure: true, ChecksumAlgorithm: Auto}
		if err := b.finalizeConfig(conf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conf.ChecksumAlgorithm != Auto {
			t.Fatalf("expected auto algorithm preserved")
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

	t.Run("missingTLSHyphenated", func(t *testing.T) {
		b := &builder{defaults: defaults, v: viper.New()}
		b.v.Set("grpc-listen", ":1")
		conf := &Config{AllowInsecure: false, ChecksumAlgorithm: "sha256"}
		if err := b.finalizeConfig(conf); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("validTLSHyphenated", func(t *testing.T) {
		dir := t.TempDir()
		cert := filepath.Join(dir, "cert.pem")
		key := filepath.Join(dir, "key.pem")
		if err := os.WriteFile(cert, []byte("cert"), 0o644); err != nil {
			t.Fatalf("write cert: %v", err)
		}
		if err := os.WriteFile(key, []byte("key"), 0o644); err != nil {
			t.Fatalf("write key: %v", err)
		}
		b := &builder{defaults: defaults, v: viper.New()}
		b.v.Set("grpc-connect", ":1")
		conf := &Config{AllowInsecure: false, TLSCert: cert, TLSKey: key, ChecksumAlgorithm: "sha256"}
		if err := b.finalizeConfig(conf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conf.GRPCConnect != ":1" {
			t.Fatalf("expected GRPCConnect populated from hyphenated key")
		}
	})
}

func TestConfigValidate(t *testing.T) {
	base := Config{
		Mode:                 "default",
		LVMEscalation:        "sudo -n",
		SSHKeepAliveInterval: time.Second,
		GRPCDialTimeout:      time.Second,
		GRPCSetupTimeout:     time.Second,
		HeartbeatInterval:    time.Second,
		HeartbeatSendTimeout: time.Second,
		TCPParallel:          1,
		CDCMin:               64,
		CDCAvg:               128,
		CDCMax:               256,
		Compress:             Auto,
		ZstdLevel:            1,
		LZ4Level:             "fast",
		CompressThreshold:    0.9,
	}
	t.Run("success", func(t *testing.T) {
		fb := &fakeBackend{free: 100}
		restore := lvm.SetBackend(fb)
		defer restore()
		restorePriv := lvm.SetPrivilegeChecker(func() error { return nil })
		defer restorePriv()
		cfg := base
		cfg.VolumeGroup = "vg0"
		cfg.LVMTimeout = time.Second
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
		cfg := base
		cfg.VolumeGroup = "vg0"
		cfg.LVMTimeout = time.Second
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidKeepalive", func(t *testing.T) {
		cfg := base
		cfg.SSHKeepAliveInterval = 0
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidHeartbeatInterval", func(t *testing.T) {
		cfg := base
		cfg.HeartbeatInterval = 0
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidHeartbeatSendTimeout", func(t *testing.T) {
		cfg := base
		cfg.HeartbeatSendTimeout = 0
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidGRPCSetupTimeout", func(t *testing.T) {
		cfg := base
		cfg.GRPCSetupTimeout = 0
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidTCPParallel", func(t *testing.T) {
		cfg := base
		cfg.TCPParallel = 5
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalidTCPLowat", func(t *testing.T) {
		cfg := base
		cfg.TCPNotSentLowAt = -1
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		restore := lvm.SetBackend(timeoutBackend{})
		defer restore()
		restorePriv := lvm.SetPrivilegeChecker(func() error { return nil })
		defer restorePriv()
		cfg := base
		cfg.VolumeGroup = "vg0"
		cfg.LVMTimeout = time.Millisecond
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected timeout error")
		}
	})
}

func TestConfigValidateCDC(t *testing.T) {
	base := Config{
		Mode:                 "default",
		SSHKeepAliveInterval: time.Second,
		GRPCDialTimeout:      time.Second,
		GRPCSetupTimeout:     time.Second,
		HeartbeatInterval:    time.Second,
		HeartbeatSendTimeout: time.Second,
		TCPParallel:          1,
		LVMEscalation:        "sudo -n",
		Compress:             Auto,
		ZstdLevel:            1,
		LZ4Level:             "fast",
		CompressThreshold:    0.9,
	}

	t.Run("valid", func(t *testing.T) {
		cfg := base
		cfg.CDCMin = 1
		cfg.CDCAvg = 1
		cfg.CDCMax = 1
		cfg.LVMEscalation = "sudo -n"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("nonPositive", func(t *testing.T) {
		cfg := base
		cfg.CDCMin = 0
		cfg.CDCAvg = 1
		cfg.CDCMax = 1
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("unordered", func(t *testing.T) {
		cfg := base
		cfg.CDCMin = 4
		cfg.CDCAvg = 2
		cfg.CDCMax = 3
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestConfigValidateCompression(t *testing.T) {
	base := Config{
		Mode:                 "default",
		LVMEscalation:        "sudo -n",
		SSHKeepAliveInterval: time.Second,
		GRPCDialTimeout:      time.Second,
		GRPCSetupTimeout:     time.Second,
		HeartbeatInterval:    time.Second,
		HeartbeatSendTimeout: time.Second,
		TCPParallel:          1,
		CDCMin:               64,
		CDCAvg:               128,
		CDCMax:               256,
		Compress:             Auto,
		ZstdLevel:            1,
		LZ4Level:             "fast",
		CompressThreshold:    0.9,
	}
	t.Run("unknownAlgorithm", func(t *testing.T) {
		cfg := base
		cfg.Compress = "gzip"
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("negativeThreshold", func(t *testing.T) {
		cfg := base
		cfg.CompressThreshold = -0.1
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("zstdLevelOutOfRange", func(t *testing.T) {
		cfg := base
		cfg.Compress = Zstd
		cfg.ZstdLevel = 6
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("lz4LevelInvalid", func(t *testing.T) {
		cfg := base
		cfg.Compress = "lz4"
		cfg.LZ4Level = "slow"
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("valid", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
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
	b := &builder{v: v, defaults: defaults}
	if err := b.applyDefaults(conf); err != nil {
		t.Fatalf("applyDefaults returned error: %v", err)
	}
	if conf.Transport != "tcp+tls" {
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
	if conf.CheckpointBytes != 1000000000 {
		t.Fatalf("checkpoint bytes %d", conf.CheckpointBytes)
	}
	if conf.CheckpointInterval != 10*time.Second {
		t.Fatalf("checkpoint interval %v", conf.CheckpointInterval)
	}
}

func TestValidateEscalationCommand(t *testing.T) {
	geteuid := func() int { return 1000 }

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	exe := filepath.Join(dir, "sudo")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create dummy executable: %v", err)
	}
	os.Setenv("PATH", dir)

	base := &Config{Mode: "default", SSHKeepAliveInterval: time.Second, GRPCDialTimeout: time.Second, GRPCSetupTimeout: time.Second, HeartbeatInterval: time.Second, HeartbeatSendTimeout: time.Second, TCPParallel: 1, CDCMin: 64, CDCAvg: 128, CDCMax: 256}

	cfg := *base
	cfg.LVMEscalation = "sudo -n"
	if err := (&cfg).ValidateWith(geteuid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg = *base
	cfg.LVMEscalation = "sudo"
	if err := (&cfg).ValidateWith(geteuid); err == nil {
		t.Fatalf("expected error without -n")
	}

	os.Setenv("PATH", "")
	cfg = *base
	cfg.LVMEscalation = "sudo -n"
	if err := (&cfg).ValidateWith(geteuid); err == nil {
		t.Fatalf("expected error when command missing")
	}
}

func TestValidateMode(t *testing.T) {
	geteuid := func() int { return 0 }
	base := Config{
		SSHKeepAliveInterval: time.Second,
		GRPCDialTimeout:      time.Second,
		GRPCSetupTimeout:     time.Second,
		HeartbeatInterval:    time.Second,
		HeartbeatSendTimeout: time.Second,
		TCPParallel:          1,
		CDCMin:               64,
		CDCAvg:               128,
		CDCMax:               256,
		LVMEscalation:        "sudo -n",
	}

	cases := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "default", mode: "default", wantErr: false},
		{name: "throughput", mode: "throughput", wantErr: false},
		{name: "invalid", mode: "fast", wantErr: true},
		{name: "empty", mode: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.Mode = tc.mode
			if err := cfg.ValidateWith(geteuid); (err != nil) != tc.wantErr {
				t.Fatalf("ValidateWith(%q) error = %v, wantErr %v", tc.mode, err, tc.wantErr)
			}
		})
	}
}

func TestValidateChecksumAuto(t *testing.T) {
	geteuid := func() int { return 0 }
	base := Config{
		Mode:                 "default",
		SSHKeepAliveInterval: time.Second,
		GRPCDialTimeout:      time.Second,
		GRPCSetupTimeout:     time.Second,
		HeartbeatInterval:    time.Second,
		HeartbeatSendTimeout: time.Second,
		TCPParallel:          1,
		CDCMin:               64,
		CDCAvg:               128,
		CDCMax:               256,
		ChecksumAlgorithm:    Auto,
	}

	t.Run("simd", func(t *testing.T) {
		cfg := base
		orig := digest.Select
		digest.Select = func() string { return digest.BLAKE3 }
		defer func() { digest.Select = orig }()
		if err := cfg.ValidateWith(geteuid); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ChecksumAlgorithm != digest.BLAKE3 {
			t.Fatalf("expected %s, got %s", digest.BLAKE3, cfg.ChecksumAlgorithm)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		cfg := base
		orig := digest.Select
		digest.Select = func() string { return digest.SHA256 }
		defer func() { digest.Select = orig }()
		if err := cfg.ValidateWith(geteuid); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ChecksumAlgorithm != digest.SHA256 {
			t.Fatalf("expected %s, got %s", digest.SHA256, cfg.ChecksumAlgorithm)
		}
	})
}

func TestValidateCDCOrdering(t *testing.T) {
	geteuid := func() int { return 0 }
	base := Config{
		Mode:                 "default",
		SSHKeepAliveInterval: time.Second,
		GRPCDialTimeout:      time.Second,
		GRPCSetupTimeout:     time.Second,
		HeartbeatInterval:    time.Second,
		HeartbeatSendTimeout: time.Second,
		TCPParallel:          1,
		LVMEscalation:        "sudo -n",
	}

	cases := []struct {
		name    string
		min     int
		avg     int
		max     int
		wantErr bool
	}{
		{name: "ascending", min: 64, avg: 128, max: 256, wantErr: false},
		{name: "avgEqualsMin", min: 64, avg: 64, max: 128, wantErr: false},
		{name: "avgEqualsMax", min: 64, avg: 128, max: 128, wantErr: false},
		{name: "avgLessThanMin", min: 128, avg: 64, max: 256, wantErr: true},
		{name: "avgGreaterThanMax", min: 64, avg: 256, max: 128, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.CDCMin, cfg.CDCAvg, cfg.CDCMax = tc.min, tc.avg, tc.max
			if err := cfg.ValidateWith(geteuid); (err != nil) != tc.wantErr {
				t.Fatalf("ValidateWith min=%d avg=%d max=%d error = %v, wantErr %v", tc.min, tc.avg, tc.max, err, tc.wantErr)
			}
		})
	}
}

func TestBuildBlockSize(t *testing.T) {
	t.Run(Auto, func(t *testing.T) {
		v := viper.New()
		v.Set("block-size", Auto)
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
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
		v.Set("block-size", "8KB")
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
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
		v.Set("zstd-level", 3)
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run(Zstd+"Invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", Zstd)
		v.Set("zstd-level", 6)
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
		if _, err := b.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run(Auto+"Valid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", Auto)
		if compressiondetect.DetectOptimalCompression() == Zstd {
			v.Set("zstd-level", 2)
		} else {
			v.Set("lz4-level", "fast")
		}
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run(Auto+"Invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", Auto)
		if compressiondetect.DetectOptimalCompression() == Zstd {
			v.Set("zstd-level", 6)
		} else {
			v.Set("lz4-level", "bad")
		}
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
		if _, err := b.Build(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("lz4Valid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "lz4")
		v.Set("lz4-level", "hc")
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("lz4Invalid", func(t *testing.T) {
		v := viper.New()
		v.Set("compress", "lz4")
		v.Set("lz4-level", "slow")
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
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
		b := &builder{v: v, defaults: defaults}
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
		v.Set("compress-concurrency", 8)
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := &builder{v: v, defaults: defaults}
		cfg, err := b.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CompressConcurrency != 8 {
			t.Fatalf("expected 8, got %d", cfg.CompressConcurrency)
		}
	})
}

func TestCompressionParsingErrors(t *testing.T) {
	t.Run("flagNegativeThreshold", func(t *testing.T) {
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := NewBuilder(defaults)
		fs, args := newFlagSet([]string{"--compress-threshold", "-0.5"})
		if _, _, _, err := b.Build(fs, args); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("envZstdLevelInvalid", func(t *testing.T) {
		defaults, err := DefaultConfig()
		if err != nil {
			t.Fatalf("DefaultConfig returned error: %v", err)
		}
		b := NewBuilder(defaults)
		t.Setenv("LVMSYNC_COMPRESSION_COMPRESS", "zstd")
		t.Setenv("LVMSYNC_COMPRESSION_ZSTD_LEVEL", "6")
		fs, args := newFlagSet(nil)
		if _, _, _, err := b.Build(fs, args); err == nil {
			t.Fatalf("expected error")
		}
	})

	// YAML parsing errors are covered by builder tests elsewhere.
}

func TestTLSFileValidation(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}

	t.Run("insecure", func(t *testing.T) {
		v := viper.New()
		v.Set("allow-insecure", true)
		b := &builder{v: v, defaults: defaults}
		if _, err := b.Build(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missingFiles", func(t *testing.T) {
		v := viper.New()
		v.Set("allow-insecure", false)
		v.Set("grpc-listen", ":1")
		b := &builder{v: v, defaults: defaults}
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
		v.Set("allow-insecure", false)
		v.Set("grpc-listen", ":1")
		v.Set("tls-cert", cert)
		v.Set("tls-key", key)
		v.Set("ca-cert", ca)
		b := &builder{v: v, defaults: defaults}
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
	rootFS, args := newFlagSet([]string{"--config", cfgPath, "--parallel", "3"})
	t.Setenv("LVMSYNC-PARALLEL", "2")
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	builder := NewBuilder(defaults)
	conf, _, _, err := builder.Build(rootFS, args)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if conf.Parallel != 3 {
		t.Fatalf("expected parallel 3, got %d", conf.Parallel)
	}
}

func TestLoadConfigInvalidPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	rootFS, args := newFlagSet([]string{"--config", missing})

	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	builder := NewBuilder(defaults)
	if _, _, _, err := builder.Build(rootFS, args); err == nil || !strings.Contains(err.Error(), "error reading config file") {
		t.Fatalf("expected config file error, got %v", err)
	}
}

func TestUnsupportedChecksumAlgorithm(t *testing.T) {
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	v := viper.New()
	v.Set("digest", "blake3-512")
	b := &builder{v: v, defaults: defaults}
	if _, err := b.Build(); err == nil {
		t.Fatalf("expected error for unsupported checksum algorithm")
	}
}
