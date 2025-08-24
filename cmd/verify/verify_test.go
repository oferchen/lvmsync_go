package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bou.ke/monkey"
	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gopkg.in/yaml.v3"

	device "lvmsync_go/device"
	"lvmsync_go/internal/config"
	privilege "lvmsync_go/internal/privilege"
	manifestpkg "lvmsync_go/manifest"
)

type syncTrackerCore struct {
	zapcore.Core
	syncs *int
}

func (s syncTrackerCore) Sync() error {
	*s.syncs++
	_ = s.Core.Sync()
	return fmt.Errorf("sync error")
}

func createTestFile(t testing.TB, size int) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "verify")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	data := bytes.Repeat([]byte{1}, size)
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return f.Name()
}

// newStubRunner returns a Runner with a no-op rebuild function.
func newStubRunner() *Runner {
	return NewRunnerWithDeps(func(_ context.Context, _ string, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		return nil
	})
}

func createManifest(t testing.TB, file string) {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	man := file + ".manifest"
	idx, err := manifestpkg.Create(man, "", uint64(len(data)), 0, 0, 0, uint32(len(data)), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	digest := blake3.Sum256(data)
	if err := idx.Set(0, uint32(len(data)), 0, 0, digest); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("manifest close: %v", err)
	}
}

func TestRunSyncsLogger(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	var syncs int
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(syncTrackerCore{Core: core, syncs: &syncs})
	r := newStubRunner()
	if err := r.Run([]string{"--dry-run", src, "dst"}, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if syncs != 1 {
		t.Fatalf("expected logger.Sync called once, got %d", syncs)
	}
	if logs.FilterMessage("logger_sync_error").Len() != 1 {
		t.Fatalf("expected sync error log")
	}
}

func TestRunLogsConfigurationWarning(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := t.TempDir() + "/dst"
	if err := os.WriteFile(dst, []byte("data"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	t.Setenv("LVMSYNC_SSH_HOST", "example")
	r := newStubRunner()
	core, obs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	if err := r.Run([]string{"--dry-run", src, dst}, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries := obs.FilterMessage("configuration_warning").All()
	if len(entries) != 1 {
		t.Fatalf("expected configuration warning, got %d", len(entries))
	}
	ctx := entries[0].ContextMap()
	if v, ok := ctx["config_key"].(string); !ok || v != "ssh-host" {
		t.Fatalf("unexpected config_key: %v", ctx["config_key"])
	}
	if v, ok := ctx["reason"].(string); !ok || v != "unknown_config_key" {
		t.Fatalf("unexpected reason: %v", ctx["reason"])
	}
}

func TestRunFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("foo"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := t.TempDir() + "/dst"
	if err := os.WriteFile(dst, []byte("bar"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	manifestPath := src + ".manifest"
	idx, err := manifestpkg.Create(manifestPath, "", uint64(len("foo")), 0, 0, 0, uint32(len("foo")), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	digest := blake3.Sum256([]byte("foo"))
	if err := idx.Set(0, uint32(len("foo")), 0, 0, digest); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("manifest close: %v", err)
	}
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("dry_run: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_DRY_RUN", "true")
	r := newStubRunner()
	err = r.Run([]string{"--config", cfgFile, "--dry-run=false", "--digest", "blake3", src, dst}, zap.NewNop())
	if err == nil {
		t.Fatalf("expected verification error")
	}
}

func TestVerifyInlineAllocations(t *testing.T) {
	blockSize := 1024
	size := blockSize * 4
	src := createTestFile(t, size)
	dst := createTestFile(t, size)
	cfg := &config.Config{BlockSize: blockSize, ChecksumAlgorithm: "blake3"}
	allocs := testing.AllocsPerRun(10, func() {
		if err := verifyInline(cfg, src, dst, zap.NewNop()); err != nil {
			t.Fatalf("verifyInline: %v", err)
		}
	})
	if allocs >= 50 {
		t.Fatalf("expected fewer allocations, got %f", allocs)
	}
}

func TestVerifyInlineSHA256(t *testing.T) {
	blockSize := 1024
	size := blockSize * 2
	src := createTestFile(t, size)
	dst := createTestFile(t, size)
	cfg := &config.Config{BlockSize: blockSize, ChecksumAlgorithm: "sha256"}
	if err := verifyInline(cfg, src, dst, zap.NewNop()); err != nil {
		t.Fatalf("verifyInline: %v", err)
	}
}

func TestVerifyWithManifestAllocations(t *testing.T) {
	blockSize := 512
	size := blockSize * 3
	src := createTestFile(t, size)
	manifestPath := filepath.Join(t.TempDir(), "manifest")
	idx, err := manifestpkg.Create(manifestPath, "", uint64(size), 0, 0, 0, uint32(blockSize), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	fSrc, err := os.Open(src)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	buf := make([]byte, blockSize)
	for off := 0; off < size; off += blockSize {
		n, err := fSrc.ReadAt(buf, int64(off))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		digest := blake3.Sum256(buf[:n])
		if err := idx.Set(uint64(off), uint32(n), 0, 0, digest); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	idx.Close()
	fSrc.Close()
	cfg := &config.Config{ChecksumAlgorithm: "blake3"}
	allocs := testing.AllocsPerRun(10, func() {
		if err := verifyWithManifest(cfg, src, manifestPath, zap.NewNop()); err != nil {
			t.Fatalf("verifyWithManifest: %v", err)
		}
	})
	if allocs >= 60 {
		t.Fatalf("expected fewer allocations, got %f", allocs)
	}
}

func TestVerifyWithManifestAlignment(t *testing.T) {
	blockSize := 4096
	// aligned file
	size := blockSize * 2
	src := createTestFile(t, size)
	manifestPath := filepath.Join(t.TempDir(), "manifest-aligned")
	idx, err := manifestpkg.Create(manifestPath, "dev", uint64(size), 0, 0, 0, uint32(blockSize), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	f, err := os.Open(src)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	buf := make([]byte, blockSize)
	for off := 0; off < size; off += blockSize {
		n, err := f.ReadAt(buf, int64(off))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		dig := blake3.Sum256(buf[:n])
		if err := idx.Set(uint64(off), uint32(n), 0, 0, dig); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	idx.Close()
	f.Close()
	cfg := &config.Config{ODirect: true, ChecksumAlgorithm: "blake3"}
	if err := verifyWithManifest(cfg, src, manifestPath, zap.NewNop()); err != nil {
		t.Fatalf("aligned verify: %v", err)
	}

	// misaligned file
	size = blockSize*2 + 1
	src = createTestFile(t, size)
	manifestPath = filepath.Join(t.TempDir(), "manifest-misaligned")
	idx, err = manifestpkg.Create(manifestPath, "dev", uint64(size), 0, 0, 0, uint32(blockSize), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	f, err = os.Open(src)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	for off := 0; off < size; {
		n := blockSize
		if remaining := size - off; remaining < n {
			n = remaining
		}
		nRead, err := f.ReadAt(buf[:n], int64(off))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		dig := blake3.Sum256(buf[:nRead])
		if err := idx.Set(uint64(off), uint32(nRead), 0, 0, dig); err != nil {
			t.Fatalf("Set: %v", err)
		}
		off += n
	}
	idx.Close()
	f.Close()
	if err := verifyWithManifest(cfg, src, manifestPath, zap.NewNop()); err != nil {
		t.Fatalf("misaligned verify: %v", err)
	}
}

func TestVerifyWithManifestParallel(t *testing.T) {
	blockSize := 1024
	blocks := 4
	size := blockSize * blocks
	src := createTestFile(t, size)
	manifestPath := filepath.Join(t.TempDir(), "manifest-parallel")
	idx, err := manifestpkg.Create(manifestPath, "dev", uint64(size), 0, 0, 0, uint32(blockSize), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	f, err := os.Open(src)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	buf := make([]byte, blockSize)
	for off := 0; off < size; off += blockSize {
		n, err := f.ReadAt(buf, int64(off))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		dig := blake3.Sum256(buf[:n])
		if err := idx.Set(uint64(off), uint32(n), 0, 0, dig); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	idx.Close()
	f.Close()

	orig := blake3.Sum256
	patch := monkey.Patch(blake3.Sum256, func(p []byte) [32]byte {
		time.Sleep(50 * time.Millisecond)
		return orig(p)
	})
	defer patch.Unpatch()

	cfg := &config.Config{Parallel: 1, ChecksumAlgorithm: "blake3"}
	start := time.Now()
	if err := verifyWithManifest(cfg, src, manifestPath, zap.NewNop()); err != nil {
		t.Fatalf("parallel=1: %v", err)
	}
	d1 := time.Since(start)

	cfg.Parallel = 4
	start = time.Now()
	if err := verifyWithManifest(cfg, src, manifestPath, zap.NewNop()); err != nil {
		t.Fatalf("parallel=4: %v", err)
	}
	d2 := time.Since(start)
	if d2 >= d1/2 {
		t.Fatalf("expected parallel verification faster, seq=%v par=%v", d1, d2)
	}
}

func TestRunNilLoggerPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
	}()
	r := newStubRunner()
	_ = r.Run([]string{"--dry-run", "src", "dst"}, nil)
}

func TestVerifyDevicesRebuildsManifest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("foo"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("foo"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	called := false
	r := NewRunnerWithDeps(func(ctx context.Context, _ string, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		called = true
		idx, err := manifestpkg.Create(output, "", uint64(len("foo")), 0, 0, 0, 4096, 0, 0, 0, 0)
		if err != nil {
			return err
		}
		digest := blake3.Sum256([]byte("foo"))
		if err := idx.Set(0, uint32(len("foo")), 0, 0, digest); err != nil {
			return err
		}
		return idx.Close()
	})
	cfg := &config.Config{ChecksumAlgorithm: "blake3"}
	if err := r.verifyDevices(context.Background(), cfg, src, dst, "", zap.NewNop()); err != nil {
		t.Fatalf("verifyDevices: %v", err)
	}
	if !called {
		t.Fatalf("expected rebuild invoked")
	}
}

func TestVerifyDevicesTimeout(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("foo"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("foo"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	r := NewRunnerWithDeps(func(ctx context.Context, _ string, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		<-ctx.Done()
		return ctx.Err()
	})
	cfg := &config.Config{ManifestTimeout: time.Millisecond, ChecksumAlgorithm: "blake3"}
	err := r.verifyDevices(context.Background(), cfg, src, dst, "", zap.NewNop())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestVerifyDevicesContextCancelled(t *testing.T) {
	r := newStubRunner()
	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan struct{})
	patch := monkey.Patch(device.Detect, func(ctx context.Context, _ string, _ bool, _ bool, _ string, _ string, _ string, _ string, _ time.Duration, _ time.Duration, _ privilege.Escalator, _ *zap.Logger, _ *device.Runner) (device.Device, error) {
		close(called)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	defer patch.Unpatch()
	cfg := &config.Config{ChecksumAlgorithm: "blake3"}
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.verifyDevices(ctx, cfg, "src", "dst", "", zap.NewNop())
	}()
	<-called
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestRunOutputsJSON(t *testing.T) {
	src := createTestFile(t, 1024)
	dst := createTestFile(t, 1024)
	createManifest(t, src)
	var buf bytes.Buffer
	oldStdout := os.Stdout
	pr, w, _ := os.Pipe()
	os.Stdout = w
	errCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, pr)
		errCh <- err
	}()
	r := newStubRunner()
	if err := r.Run([]string{"--output", "json", "--digest", "blake3", src, dst}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	w.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("copy: %v", err)
	}
	os.Stdout = oldStdout
	var out struct {
		Verified bool `json:"verified"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Verified {
		t.Fatalf("expected verified true")
	}
}

func TestRunOutputsYAML(t *testing.T) {
	src := createTestFile(t, 1024)
	dst := createTestFile(t, 1024)
	createManifest(t, src)
	var buf bytes.Buffer
	oldStdout := os.Stdout
	pr, w, _ := os.Pipe()
	os.Stdout = w
	errCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(&buf, pr)
		errCh <- err
	}()
	r := newStubRunner()
	if err := r.Run([]string{"--output", "yaml", "--digest", "blake3", src, dst}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	w.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("copy: %v", err)
	}
	os.Stdout = oldStdout
	var out struct {
		Verified bool `yaml:"verified"`
	}
	if err := yaml.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Verified {
		t.Fatalf("expected verified true")
	}
}

func TestVerifyWithManifestIdentityMatch(t *testing.T) {
	size := 4096
	src := createTestFile(t, size)
	manifestPath := filepath.Join(t.TempDir(), "manifest")
	idx, err := manifestpkg.Create(manifestPath, "", uint64(size), 0, 0, 0, uint32(size), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	digest := blake3.Sum256(bytes.Repeat([]byte{1}, size))
	if err := idx.Set(0, uint32(size), 0, 0, digest); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("manifest close: %v", err)
	}
	cfg := &config.Config{ChecksumAlgorithm: "blake3"}
	if err := verifyWithManifest(cfg, src, manifestPath, zap.NewNop()); err != nil {
		t.Fatalf("verifyWithManifest: %v", err)
	}
}

func TestVerifyWithManifestIdentityMismatch(t *testing.T) {
	size := 4096
	src := createTestFile(t, size)
	manifestPath := filepath.Join(t.TempDir(), "manifest")
	idx, err := manifestpkg.Create(manifestPath, "", uint64(size), 0, 1, 0, uint32(size), 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	digest := blake3.Sum256(bytes.Repeat([]byte{1}, size))
	if err := idx.Set(0, uint32(size), 0, 0, digest); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("manifest close: %v", err)
	}
	cfg := &config.Config{}
	if err := verifyWithManifest(cfg, src, manifestPath, zap.NewNop()); err != nil {
		t.Fatalf("verifyWithManifest: %v", err)
	}
}

type mockFile struct {
	logical int
	direct  bool
	closes  *int
}

func (m *mockFile) Close() error                            { *m.closes++; return nil }
func (m *mockFile) Logical() int                            { return m.logical }
func (m *mockFile) Direct() bool                            { return m.direct }
func (m *mockFile) ReadAt(p []byte, off int64) (int, error) { return len(p), nil }

func TestVerifyWithManifestClosesOnce(t *testing.T) {
	var c1, c2 int
	f1 := &mockFile{logical: 4096, direct: true, closes: &c1}
	f2 := &mockFile{logical: 4096, direct: false, closes: &c2}
	open := func(string, bool, bool) (blockReader, error) {
		if c1 == 0 {
			return f1, nil
		}
		return f2, nil
	}
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest")
	blockSize := uint32(4096)
	idx, err := manifestpkg.Create(manifestPath, "", 4097, 0, 0, 0, blockSize, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("manifest create: %v", err)
	}
	digest := blake3.Sum256(make([]byte, blockSize))
	if err := idx.Set(1, blockSize, 0, 0, digest); err != nil {
		t.Fatalf("manifest set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("manifest close: %v", err)
	}
	cfg := &config.Config{Parallel: 1, ODirect: true, BlockSize: int(blockSize)}
	if err := verifyWithManifestOpen(open, cfg, "src", manifestPath, zap.NewNop()); err != nil {
		t.Fatalf("verifyWithManifestOpen: %v", err)
	}
	if c1 != 1 {
		t.Fatalf("expected first file closed once, got %d", c1)
	}
	if c2 != 1 {
		t.Fatalf("expected reopened file closed once, got %d", c2)
	}
}
