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

	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gopkg.in/yaml.v3"

	"lvmsync_go/internal/config"
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
	return NewRunnerWithDeps(func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
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
	idx, err := manifestpkg.Create(man, "dev", uint64(len(data)), 0, 0, 0, uint32(len(data)), 0, 0, 0, 0)
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
	if logs.FilterMessage("Logger sync error").Len() != 1 {
		t.Fatalf("expected sync error log")
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
	idx, err := manifestpkg.Create(manifestPath, "dev", uint64(len("foo")), 0, 0, 0, uint32(len("foo")), 0, 0, 0, 0)
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
	err = r.Run([]string{"--config", cfgFile, "--dry-run=false", src, dst}, zap.NewNop())
	if err == nil {
		t.Fatalf("expected verification error")
	}
}

func TestVerifyFullAllocations(t *testing.T) {
	blockSize := 1024
	size := blockSize * 4
	src := createTestFile(t, size)
	dst := createTestFile(t, size)
	cfg := &config.Config{BlockSize: blockSize}
	allocs := testing.AllocsPerRun(10, func() {
		if err := verifyFull(cfg, src, dst, zap.NewNop()); err != nil {
			t.Fatalf("verifyFull: %v", err)
		}
	})
	if allocs >= 15 {
		t.Fatalf("expected fewer allocations, got %f", allocs)
	}
}

func TestVerifyFullSHA256(t *testing.T) {
	blockSize := 1024
	size := blockSize * 2
	src := createTestFile(t, size)
	dst := createTestFile(t, size)
	cfg := &config.Config{BlockSize: blockSize, ChecksumAlgorithm: "sha256"}
	if err := verifyFull(cfg, src, dst, zap.NewNop()); err != nil {
		t.Fatalf("verifyFull: %v", err)
	}
}

func TestVerifyWithManifestAllocations(t *testing.T) {
	blockSize := 512
	size := blockSize * 3
	src := createTestFile(t, size)
	manifestPath := filepath.Join(t.TempDir(), "manifest")
	idx, err := manifestpkg.Create(manifestPath, "dev", uint64(size), 0, 0, 0, uint32(blockSize), 0, 0, 0, 0)
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
	cfg := &config.Config{}
	allocs := testing.AllocsPerRun(10, func() {
		if err := verifyWithManifest(cfg, src, manifestPath, zap.NewNop()); err != nil {
			t.Fatalf("verifyWithManifest: %v", err)
		}
	})
	if allocs >= 40 {
		t.Fatalf("expected fewer allocations, got %f", allocs)
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
	r := NewRunnerWithDeps(func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		called = true
		idx, err := manifestpkg.Create(output, "dev", uint64(len("foo")), 0, 0, 0, 4096, 0, 0, 0, 0)
		if err != nil {
			return err
		}
		digest := blake3.Sum256([]byte("foo"))
		if err := idx.Set(0, uint32(len("foo")), 0, 0, digest); err != nil {
			return err
		}
		return idx.Close()
	})
	cfg := &config.Config{}
	if err := r.verifyDevices(cfg, src, dst, "", zap.NewNop()); err != nil {
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
	r := NewRunnerWithDeps(func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		<-ctx.Done()
		return ctx.Err()
	})
	cfg := &config.Config{ManifestTimeout: time.Millisecond}
	err := r.verifyDevices(cfg, src, dst, "", zap.NewNop())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout error, got %v", err)
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
	if err := r.Run([]string{"--output", "json", src, dst}, zap.NewNop()); err != nil {
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
	if err := r.Run([]string{"--output", "yaml", src, dst}, zap.NewNop()); err != nil {
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
