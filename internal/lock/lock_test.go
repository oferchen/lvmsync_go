package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	restore := SetBaseDir(dir)
	defer restore()
	l, err := Acquire("vg0", "lv0")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	path := filepath.Join(dir, "vg0.lv0.lock")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file not removed")
	}
}
