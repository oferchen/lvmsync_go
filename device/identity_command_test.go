package device

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func createSleepScript(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nsleep 2\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func createTouchScript(t *testing.T, dir, name, touch string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\ntouch " + touch + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestRawIdentityTimeout(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer f.Close()
	d := &RawDevice{f: f, logger: zap.NewNop()}
	script := createSleepScript(t, "blkid")
	orig := blkidPath
	blkidPath = script
	defer func() { blkidPath = orig }()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := d.Identity(ctx); err == nil || (!errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "killed")) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestRawIdentityIgnoresPATH(t *testing.T) {
	if blkidPath == "" {
		t.Skip("blkid not found")
	}
	f, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer f.Close()
	d := &RawDevice{f: f, logger: zap.NewNop()}
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	createTouchScript(t, dir, "blkid", marker)
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)
	_, _ = d.Identity(context.Background())
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("malicious blkid executed")
	}
}

func TestLVMIdentityTimeout(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	d := &LVMDevice{path: f.Name(), logger: zap.NewNop()}
	script := createSleepScript(t, "lvs")
	orig := lvsPath
	lvsPath = script
	defer func() { lvsPath = orig }()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := d.Identity(ctx); err == nil || (!errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "killed")) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestLVMIdentityIgnoresPATH(t *testing.T) {
	if lvsPath == "" {
		t.Skip("lvs not found")
	}
	f, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	d := &LVMDevice{path: f.Name(), logger: zap.NewNop()}
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	createTouchScript(t, dir, "lvs", marker)
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)
	_, _ = d.Identity(context.Background())
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("malicious lvs executed")
	}
}
