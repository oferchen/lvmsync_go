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
	_ = d.Cleanup(ctx)
	d.Close()

	if d.SizeBytes() != 4096 {
		t.Fatalf("expected size 4096, got %d", d.SizeBytes())
	}
	if d.BlockSize() == 0 {
		t.Fatalf("expected non-zero block size")
	}

	if logs.FilterMessage("file_device_opened").Len() == 0 {
		t.Fatalf("expected file_device_opened log")
	}
	entries := logs.FilterMessage("file_device_info").All()
	found := false
	for _, e := range entries {
		if e.ContextMap()["path"] == path &&
			e.ContextMap()["size_bytes"].(uint64) == d.SizeBytes() &&
			e.ContextMap()["block_size_bytes"].(uint64) == d.BlockSize() {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected file_device_info log with fields, got %v", logs.All())
	}
	if logs.FilterMessage("file_device_closed").Len() == 0 {
		t.Fatalf("expected file_device_closed log")
	}
}

func TestOpenFileRejectsNonRegular(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenFile(dir, zap.NewNop()); err == nil {
		t.Fatalf("expected error for directory")
	}
}

func TestOpenFileNilLogger(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	if _, err := OpenFile(f.Name(), nil); err == nil {
		t.Fatalf("expected error when logger is nil")
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

	d, err := OpenFile(path, zap.NewNop())
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

	d, err := OpenFile(path, zap.NewNop())
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

func TestFileDevicePath(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "filedev")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	path := f.Name()
	f.Close()
	d, err := OpenFile(path, zap.NewNop())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	if d.Path() != path {
		t.Fatalf("Path() = %q, want %q", d.Path(), path)
	}
}
