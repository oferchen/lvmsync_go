package lvm

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func init() {
	SetEscalationCommand("")
}

func TestGetSnapshotUsage(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })

	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "lvs")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 75.5\n"), 0755); err != nil {
		t.Fatalf("failed to write lvs script: %v", err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)

	usage, err := GetSnapshotUsage("/dev/vg0/snap")
	if err != nil {
		t.Fatalf("GetSnapshotUsage failed: %v", err)
	}
	if usage != 75.5 {
		t.Fatalf("usage = %v, want 75.5", usage)
	}
}

func TestMonitorSnapshot(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })

	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "lvs")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 90\n"), 0755); err != nil {
		t.Fatalf("failed to write lvs script: %v", err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)

	err := MonitorSnapshot("/dev/vg0/snap", 80, 10*time.Millisecond, make(chan struct{}))
	if err == nil {
		t.Fatalf("MonitorSnapshot expected error, got nil")
	}
}

func TestCheckDiskSpace(t *testing.T) {
	orig := statfsFunc
	statfsFunc = func(_ string, stat *syscall.Statfs_t) error {
		stat.Bavail = 100
		stat.Bsize = 4096
		return nil
	}
	defer func() { statfsFunc = orig }()

	available, err := CheckDiskSpace("/mnt")
	if err != nil {
		t.Fatalf("CheckDiskSpace failed: %v", err)
	}
	const expected = 100 * 4096
	if available != expected {
		t.Fatalf("available = %d, want %d", available, expected)
	}
}
