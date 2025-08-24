package wal

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAndClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// seed with dummy header bytes
	if _, err := f.Write(make([]byte, 16)); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := f.Seek(16, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	w := New(f, nil)
	if err := w.Append(Range{Start: 1, End: 2}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != 32 { // header + one entry
		t.Fatalf("unexpected size %d", len(data))
	}
}

type stubFile struct {
	*os.File
	syncErr    error
	syncCalled bool
	closed     bool
}

func (s *stubFile) Sync() error {
	s.syncCalled = true
	if s.syncErr != nil {
		return s.syncErr
	}
	return s.File.Sync()
}

func (s *stubFile) Close() error {
	s.closed = true
	return s.File.Close()
}

type shortWriteFile struct{ *stubFile }

func (s *shortWriteFile) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func TestSyncError(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "wal"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sf := &stubFile{File: f, syncErr: errors.New("sync fail")}
	w := New(sf, nil)
	if err := w.Sync(); err == nil || !strings.Contains(err.Error(), "sync fail") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sf.syncCalled {
		t.Fatalf("sync not called")
	}
}

func TestCloseDirSync(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "wal"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sf := &stubFile{File: f}
	var synced string
	deps := NewDeps()
	deps.syncDir = func(p string) error { synced = p; return nil }
	w := New(sf, deps)
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if synced != dir {
		t.Fatalf("syncDir not called with dir: %q", synced)
	}
	if w.f != nil {
		t.Fatalf("file not nil after close")
	}
	if !sf.closed {
		t.Fatalf("file not closed")
	}
}

func TestCloseFileSyncError(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "wal"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sf := &stubFile{File: f, syncErr: errors.New("sync fail")}
	var called bool
	deps := NewDeps()
	deps.syncDir = func(string) error { called = true; return nil }
	w := New(sf, deps)
	if err := w.Close(); err == nil || !strings.Contains(err.Error(), "sync fail") {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatalf("syncDir should not be called on sync error")
	}
	if !sf.closed {
		t.Fatalf("file not closed")
	}
}

func TestCloseDirSyncError(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "wal"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sf := &stubFile{File: f}
	deps := NewDeps()
	deps.syncDir = func(string) error { return errors.New("dir sync fail") }
	w := New(sf, deps)
	if err := w.Close(); err == nil || !strings.Contains(err.Error(), "dir sync fail") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sf.closed {
		t.Fatalf("file not closed")
	}
}

func TestAppendShortWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	deps := NewDeps()
	deps.openFile = func(name string, flag int, perm os.FileMode) (File, error) {
		if strings.HasSuffix(name, ".tmp") {
			tf, err := os.OpenFile(name, flag, perm)
			if err != nil {
				return nil, err
			}
			return &shortWriteFile{&stubFile{File: tf}}, nil
		}
		return os.OpenFile(name, flag, perm)
	}
	w := New(f, deps)
	if err := w.Append(Range{Start: 1, End: 2}); err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppendRenameError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	deps := NewDeps()
	deps.rename = func(string, string) error { return errors.New("rename fail") }
	w := New(f, deps)
	if err := w.Append(Range{Start: 1, End: 2}); err == nil || !strings.Contains(err.Error(), "rename fail") {
		t.Fatalf("unexpected error: %v", err)
	}
}
