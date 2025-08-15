package verify

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
	"lvmsync_go/manifest"
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

func TestRunSyncsLogger(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	var syncs int
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(syncTrackerCore{Core: core, syncs: &syncs})
	if err := Run([]string{"--dry-run", src, "dst"}, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if syncs != 1 {
		t.Fatalf("expected logger.Sync called once, got %d", syncs)
	}
	if logs.FilterMessage("Logger sync error").Len() != 1 {
		t.Fatalf("expected sync error log")
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

func TestVerifyWithManifestAllocations(t *testing.T) {
	blockSize := 512
	size := blockSize * 3
	src := createTestFile(t, size)
	manifestPath := filepath.Join(t.TempDir(), "manifest")
	idx, err := manifest.Create(manifestPath, "dev", uint64(size), uint32(blockSize))
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
		if err := idx.Set(uint64(off), uint32(n), 0, digest); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	idx.Close()
	fSrc.Close()
	allocs := testing.AllocsPerRun(10, func() {
		if err := verifyWithManifest(src, manifestPath, zap.NewNop()); err != nil {
			t.Fatalf("verifyWithManifest: %v", err)
		}
	})
	if allocs >= 40 {
		t.Fatalf("expected fewer allocations, got %f", allocs)
	}
}
