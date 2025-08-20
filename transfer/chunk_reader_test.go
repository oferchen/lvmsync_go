package transfer

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/config"
)

func TestChunkReaderSkipsConfirmed(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	tmp := t.TempDir()
	path := filepath.Join(tmp, "dev")
	data := make([]byte, defaultChunkSize*2)
	for i := range data {
		data[i] = byte(i)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	// bitmap marks first chunk as confirmed
	cr, err := NewChunkReader(path, defaultChunkSize, []byte{0x1})
	if err != nil {
		t.Fatalf("NewChunkReader: %v", err)
	}
	defer cr.Close()
	off, buf, err := cr.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if off != int64(defaultChunkSize) {
		t.Fatalf("expected offset %d, got %d", defaultChunkSize, off)
	}
	if len(buf) != defaultChunkSize {
		t.Fatalf("expected chunk size %d, got %d", defaultChunkSize, len(buf))
	}
	putAlignedBlockBuffer(buf)
	if _, _, err := cr.Next(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestChunkReaderShortRead(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "dev")
	// create file smaller than block size
	if err := os.WriteFile(path, []byte("abcd"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cr, err := NewChunkReader(path, defaultChunkSize, nil)
	if err != nil {
		t.Fatalf("NewChunkReader: %v", err)
	}
	defer cr.Close()
	if _, _, err := cr.Next(); err == nil {
		t.Fatalf("expected error on short read")
	}
}

func TestPunchHoleError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file")
	if err := os.WriteFile(path, make([]byte, 4096), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	f, err := os.Open(path) // read-only
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	if err := punchHole(f, 0, 4096); err == nil {
		t.Fatalf("expected error punching hole on read-only file")
	}
}

func TestPunchHoleENOTSUPDisables(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var calls int
	deps := *DefaultDeps
	deps.PunchHole = func(_ *os.File, _ uint64, _ int) error {
		calls++
		return unix.ENOTSUP
	}
	punchHoleDisabled.Store(false)
	defer punchHoleDisabled.Store(false)

	cfg := &config.Config{BlockSize: 4096}
	core, obs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	if err := writeZeroBlock(cfg, f, 0, logger, &deps); err != nil {
		t.Fatalf("writeZeroBlock: %v", err)
	}
	if err := writeZeroBlock(cfg, f, uint64(cfg.BlockSize), logger, &deps); err != nil {
		t.Fatalf("writeZeroBlock: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected PunchHole called once, got %d", calls)
	}
	if obs.Len() != 1 {
		t.Fatalf("expected one warning, got %d", obs.Len())
	}
}
