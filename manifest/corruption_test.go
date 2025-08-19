package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.man")
	idx, err := Create(path, "dev", 8192, 0, 4096, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := os.Truncate(path, info.Size()-1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "unexpected file size") {
		t.Fatalf("expected size error, got %v", err)
	}
}
