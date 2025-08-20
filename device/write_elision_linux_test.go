//go:build linux

package device_test

import (
	"bytes"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/config"
	transfer "lvmsync_go/transfer"
	_ "unsafe"
)

//go:linkname writeZeroBlock lvmsync_go/transfer.writeZeroBlock
func writeZeroBlock(cfg *config.Config, dest *os.File, offset uint64, logger *zap.Logger, deps *transfer.Deps) error

//go:linkname punchHoleDisabled lvmsync_go/transfer.punchHoleDisabled
var punchHoleDisabled atomic.Bool

func TestWriteElisionFallocateSupported(t *testing.T) {
	cfg := &config.Config{BlockSize: 4096}
	f, err := os.CreateTemp(t.TempDir(), "elide")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	data := bytes.Repeat([]byte{1}, cfg.BlockSize*2)
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := writeZeroBlock(cfg, f, uint64(cfg.BlockSize), zap.NewNop(), transfer.DefaultDeps); err != nil {
		t.Fatalf("writeZeroBlock: %v", err)
	}

	if off, err := unix.Seek(int(f.Fd()), int64(cfg.BlockSize), unix.SEEK_DATA); !errors.Is(err, unix.ENXIO) {
		t.Fatalf("expected hole after punching, off=%d err=%v", off, err)
	}
}

func TestWriteElisionFallocateUnsupported(t *testing.T) {
	cfg := &config.Config{BlockSize: 4096}
	f, err := os.CreateTemp(t.TempDir(), "elide")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	data := bytes.Repeat([]byte{1}, cfg.BlockSize*2)
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}

	var calls int
	deps := *transfer.DefaultDeps
	deps.PunchHole = func(*os.File, uint64, int) error {
		calls++
		return unix.ENOTSUP
	}
	punchHoleDisabled.Store(false)
	defer punchHoleDisabled.Store(false)

	if err := writeZeroBlock(cfg, f, uint64(cfg.BlockSize), zap.NewNop(), &deps); err != nil {
		t.Fatalf("writeZeroBlock: %v", err)
	}
	if err := writeZeroBlock(cfg, f, 0, zap.NewNop(), &deps); err != nil {
		t.Fatalf("writeZeroBlock: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected PunchHole called once, got %d", calls)
	}

	buf := make([]byte, cfg.BlockSize)
	if _, err := f.ReadAt(buf, int64(cfg.BlockSize)); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf, make([]byte, cfg.BlockSize)) {
		t.Fatalf("expected zero block written")
	}
	if off, err := unix.Seek(int(f.Fd()), int64(cfg.BlockSize), unix.SEEK_DATA); err != nil || off != int64(cfg.BlockSize) {
		t.Fatalf("expected data at %d after fallback, off=%d err=%v", cfg.BlockSize, off, err)
	}
}
