package lvm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

func TestNewFDCacheNilLoggerError(t *testing.T) {
	if _, err := NewFDCache(fdCacheSize, nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSetLoggerNilError(t *testing.T) {
	cache, err := NewFDCache(fdCacheSize, zap.NewNop())
	if err != nil {
		t.Fatalf("NewFDCache: %v", err)
	}
	if err := cache.SetLogger(nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFDCacheEvictionOrder(t *testing.T) {
	cache, err := NewFDCache(fdCacheSize, zap.NewNop())
	if err != nil {
		t.Fatalf("NewFDCache: %v", err)
	}

	tmpDir := t.TempDir()
	var fd0, fd1 int
	for i := 0; i < fdCacheSize; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%d", i))
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
		fd, err := cache.getFD(path)
		if err != nil {
			t.Fatalf("getFD %d: %v", i, err)
		}
		if i == 0 {
			fd0 = fd
		}
		if i == 1 {
			fd1 = fd
		}
	}

	if _, err := cache.getFD(filepath.Join(tmpDir, "file0")); err != nil {
		t.Fatalf("reaccess file0: %v", err)
	}

	extra := filepath.Join(tmpDir, "extra")
	if err := os.WriteFile(extra, nil, 0644); err != nil {
		t.Fatalf("write extra: %v", err)
	}
	if _, err := cache.getFD(extra); err != nil {
		t.Fatalf("getFD extra: %v", err)
	}

	if _, err := unix.FcntlInt(uintptr(fd1), unix.F_GETFD, 0); err == nil {
		t.Fatalf("expected fd1 to be closed")
	}
	if _, err := unix.FcntlInt(uintptr(fd0), unix.F_GETFD, 0); err != nil {
		t.Fatalf("expected fd0 to remain open: %v", err)
	}

	cache.Close()
}

func TestFDCacheCloseClosesAll(t *testing.T) {
	cache, err := NewFDCache(fdCacheSize, zap.NewNop())
	if err != nil {
		t.Fatalf("NewFDCache: %v", err)
	}
	tmpDir := t.TempDir()
	var fds []int
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%d", i))
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
		fd, err := cache.getFD(path)
		if err != nil {
			t.Fatalf("getFD %d: %v", i, err)
		}
		fds = append(fds, fd)
	}

	cache.Close()

	for i, fd := range fds {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
			t.Fatalf("fd %d still open", i)
		}
	}
}

func TestGetVolumeSizeCachesFD(t *testing.T) {
	cache, err := NewDeviceFDCache(zap.NewNop())
	if err != nil {
		t.Fatalf("NewDeviceFDCache: %v", err)
	}
	defer cache.Close()

	tmpFile := filepath.Join(t.TempDir(), "vol")
	if err := os.WriteFile(tmpFile, make([]byte, 1024), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	size, err := GetVolumeSize(tmpFile, cache, zap.NewNop())
	if err != nil {
		t.Fatalf("GetVolumeSize failed: %v", err)
	}
	if size != 1024 {
		t.Fatalf("size = %d, want 1024", size)
	}

	elem, ok := cache.fds[tmpFile]
	if !ok {
		t.Fatalf("file descriptor not cached")
	}
	entry, ok := elem.Value.(*fdCacheEntry)
	if !ok {
		t.Fatalf("invalid cache entry type")
	}
	fd := entry.fd

	size, err = GetVolumeSize(tmpFile, cache, zap.NewNop())
	if err != nil {
		t.Fatalf("second GetVolumeSize failed: %v", err)
	}
	if size != 1024 {
		t.Fatalf("size = %d, want 1024", size)
	}

	elem2, ok := cache.fds[tmpFile]
	if !ok {
		t.Fatalf("file descriptor not reused")
	}
	entry2, ok := elem2.Value.(*fdCacheEntry)
	if !ok || entry2.fd != fd {
		t.Fatalf("file descriptor not reused")
	}
}

func TestFDCLOEXEC(t *testing.T) {
	cache, err := NewDeviceFDCache(zap.NewNop())
	if err != nil {
		t.Fatalf("NewDeviceFDCache: %v", err)
	}
	defer cache.Close()

	tmpFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(tmpFile, nil, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	fd, err := cache.getFD(tmpFile)
	if err != nil {
		t.Fatalf("getFD: %v", err)
	}
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if err != nil {
		t.Fatalf("fcntl: %v", err)
	}
	if flags&unix.FD_CLOEXEC == 0 {
		t.Fatalf("FD_CLOEXEC not set")
	}
}
