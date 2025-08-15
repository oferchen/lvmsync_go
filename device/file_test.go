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
	if logs.FilterMessage("file device closed").Len() == 0 {
		t.Fatalf("expected file device closed log")
	}
}

func TestOpenFileRejectsNonRegular(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenFile(dir, nil); err == nil {
		t.Fatalf("expected error for directory")
	}
}

func fdCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	return len(entries)
}

func TestFileSnapshotIdentityAndFDLeak(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "filedev")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	path := f.Name()
	f.Close()

	d, err := OpenFile(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	before := fdCount(t)
	snap, err := d.Snapshot(context.Background(), "")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap != d {
		t.Fatalf("snapshot returned different device")
	}
	after := fdCount(t)
	if before != after {
		t.Fatalf("fd leak: before %d after %d", before, after)
	}
}

func TestFileSnapshotClosedDevice(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "filedev")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	path := f.Name()
	f.Close()

	d, err := OpenFile(path, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := d.Snapshot(context.Background(), ""); err == nil {
		t.Fatalf("expected error for closed device")
	}
}
