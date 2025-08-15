package device

import (
	"context"
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestOpenFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "filedev")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := f.Write(make([]byte, 4096)); err != nil {
		t.Fatalf("write: %v", err)
	}
	path := f.Name()
	f.Close()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	d, err := OpenFile(path, logger)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	_, _ = d.Snapshot(ctx, "")
	_ = d.Cleanup(ctx, "", nil)
	d.Close()

	if d.SizeBytes() != 4096 {
		t.Fatalf("expected size 4096, got %d", d.SizeBytes())
	}
	if d.BlockSize() == 0 {
		t.Fatalf("expected non-zero block size")
	}

	if logs.FilterMessage("file device opened").Len() == 0 {
		t.Fatalf("expected file device opened log")
	}
	if logs.FilterMessage("file device info").Len() == 0 {
		t.Fatalf("expected file device info log")
	}
	if logs.FilterMessage("file device snapshot").Len() == 0 {
		t.Fatalf("expected file device snapshot log")
	}
	if logs.FilterMessage("file device cleanup").Len() == 0 {
		t.Fatalf("expected file device cleanup log")
	}
	if logs.FilterMessage("file device closed").Len() == 0 {
		t.Fatalf("expected file device closed log")
	}
}

func TestOpenFileRejectsNonRegular(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenFile(dir, zap.NewNop()); err == nil {
		t.Fatalf("expected error for directory")
	}
}
