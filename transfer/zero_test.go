package transfer

import (
	"bytes"
	"os"
	"testing"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/config"
)

func TestIsAllZero(t *testing.T) {
	if !isAllZero([]byte{0, 0, 0}) {
		t.Fatalf("expected zero slice to be detected")
	}
	if isAllZero([]byte{0, 1, 0}) {
		t.Fatalf("expected non-zero slice to be detected")
	}
}

func TestZeroHashLargeBuffer(t *testing.T) {
	size := 4096
	buf := make([]byte, size)
	if !isAllZero(buf) {
		t.Fatalf("expected zero buffer to be detected")
	}
	expected := blake3.Sum256(buf)
	if got := zeroHash(size); got != expected {
		t.Fatalf("unexpected zero hash")
	}
	buf[123] = 1
	if isAllZero(buf) {
		t.Fatalf("expected detection of non-zero buffer")
	}
}

func TestWriteZeroBlockPunchesHole(t *testing.T) {
	cfg := &config.Config{BlockSize: 4096, ODirect: true}
	tmp := t.TempDir()
	path := tmp + "/hole"
	f, direct, err := openFileODirect(path, os.O_RDWR|os.O_CREATE)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !direct {
		t.Skip("O_DIRECT unsupported")
	}
	defer f.Close()

	data := bytes.Repeat([]byte{1}, cfg.BlockSize*2)
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := writeZeroBlock(cfg, f, uint64(cfg.BlockSize), zap.NewNop(), DefaultDeps); err != nil {
		t.Fatalf("writeZeroBlock: %v", err)
	}

	buf := make([]byte, cfg.BlockSize)
	if _, err := f.ReadAt(buf, int64(cfg.BlockSize)); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !isAllZero(buf) {
		t.Fatalf("expected zeros after punching hole")
	}
}

func TestSetupSourceFileWarnsOnFallback(t *testing.T) {
	cfg := &config.Config{BlockSize: 4096, ODirect: true}
	tmp := t.TempDir()
	path := tmp + "/src"
	if err := os.WriteFile(path, make([]byte, cfg.BlockSize), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, direct, err := openFileODirect(path, os.O_RDONLY)
	if err != nil {
		t.Fatalf("openFileODirect: %v", err)
	}
	_ = f.Close()
	if direct {
		t.Skip("O_DIRECT supported")
	}
	core, obs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	src, err := setupSourceFile(cfg, path, logger)
	if err != nil {
		t.Fatalf("setupSourceFile: %v", err)
	}
	_ = src.Close()
	if obs.FilterMessage("odirect_requested_but_unused").Len() != 1 {
		t.Fatalf("expected warning log when falling back")
	}
}
