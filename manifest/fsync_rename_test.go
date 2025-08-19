package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFsyncRenameCrash simulates a power loss immediately after os.Rename
// but before the directory is fsynced. The manifest should contain either the
// old or new contents, never a mix of both.
func TestFsyncRenameCrash(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "manifest")
	tmp := filepath.Join(dir, "tmp")

	oldData := []byte("old")
	newData := []byte("newer")
	if err := os.WriteFile(final, oldData, 0o600); err != nil {
		t.Fatalf("write final: %v", err)
	}
	if err := os.WriteFile(tmp, newData, 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	// Rename without syncing the directory to simulate a crash before
	// fsync. Depending on the filesystem, either the old or new data may
	// be observed after recovery, but it must be one of them and not a
	// partial file.
	if err := os.Rename(tmp, final); err != nil {
		t.Fatalf("rename: %v", err)
	}

	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if string(data) != string(oldData) && string(data) != string(newData) {
		t.Fatalf("unexpected data %q", data)
	}
}

// TestFsyncRename ensures the helper flushes the directory so the rename is
// durable once the function returns.
func TestFsyncRename(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "manifest")
	tmp := filepath.Join(dir, "tmp")

	if err := os.WriteFile(final, []byte("old"), 0o600); err != nil {
		t.Fatalf("write final: %v", err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := fsyncRename(tmp, final); err != nil {
		t.Fatalf("fsyncRename: %v", err)
	}
	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("rename not applied: %q", data)
	}
}
