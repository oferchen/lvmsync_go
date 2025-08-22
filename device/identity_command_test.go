package device

import (
	"context"
	"errors"
	"fmt"
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
	origBLKID := blkidPath
	origLSBLK := lsblkPath
	origBLKIDErr := blkidErr
	origLSBLKErr := lsblkErr
	blkidPath = script
	lsblkPath = script
	blkidErr = nil
	lsblkErr = nil
	defer func() {
		blkidPath = origBLKID
		lsblkPath = origLSBLK
		blkidErr = origBLKIDErr
		lsblkErr = origLSBLKErr
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := d.Identity(ctx); err == nil || !(errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "signal: killed") || strings.Contains(err.Error(), "exit status")) {
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
	origErr := lvsErr
	lvsPath = script
	lvsErr = nil
	defer func() { lvsPath = orig; lvsErr = origErr }()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := d.Identity(ctx); err == nil || !(errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "signal: killed")) {
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

func TestRawIdentityMissingDependency(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer f.Close()
	d := &RawDevice{f: f, logger: zap.NewNop()}
	origPath := blkidPath
	origErr := blkidErr
	blkidPath = ""
	blkidErr = fmt.Errorf("blkid: %w", ErrDependencyMissing)
	defer func() {
		blkidPath = origPath
		blkidErr = origErr
	}()
	if _, err := d.Identity(context.Background()); err == nil || !errors.Is(err, ErrDependencyMissing) || !strings.Contains(err.Error(), "blkid") {
		t.Fatalf("expected blkid missing error, got %v", err)
	}
}

func TestLVMIdentityMissingDependency(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	d := &LVMDevice{path: f.Name(), logger: zap.NewNop()}
	origPath := lvsPath
	origErr := lvsErr
	lvsPath = ""
	lvsErr = fmt.Errorf("lvs: %w", ErrDependencyMissing)
	defer func() {
		lvsPath = origPath
		lvsErr = origErr
	}()
	if _, err := d.Identity(context.Background()); err == nil || !errors.Is(err, ErrDependencyMissing) || !strings.Contains(err.Error(), "lvs") {
		t.Fatalf("expected lvs missing error, got %v", err)
	}
}
