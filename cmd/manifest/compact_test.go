// Package manifest tests manifest CLI compact.
package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
	manifestpkg "github.com/oferchen/lvmsync_go/manifest"
)

const compactEntrySize = 56

func TestRunCompactReordersEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.man")
	idx, err := manifestpkg.Create(path, "dev", 8192, 0, 0, 0, 4096, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	a := []byte("aaaa")
	b := []byte("bbbb")
	if err := idx.Set(0, uint32(len(a)), 0, xxh3.Hash(a), blake3.Sum256(a)); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if err := idx.Set(4096, uint32(len(b)), 0, xxh3.Hash(b), blake3.Sum256(b)); err != nil {
		t.Fatalf("set b: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	buf0 := make([]byte, compactEntrySize)
	buf1 := make([]byte, compactEntrySize)
	if _, err := f.ReadAt(buf0, int64(manifestpkg.HeaderSize)); err != nil {
		t.Fatalf("read0: %v", err)
	}
	if _, err := f.ReadAt(buf1, int64(manifestpkg.HeaderSize+compactEntrySize)); err != nil {
		t.Fatalf("read1: %v", err)
	}
	if _, err := f.WriteAt(buf0, int64(manifestpkg.HeaderSize+compactEntrySize)); err != nil {
		t.Fatalf("write0: %v", err)
	}
	if _, err := f.WriteAt(buf1, int64(manifestpkg.HeaderSize)); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if err := Run(cfg, []string{"compact", path}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	idx2, err := manifestpkg.Open(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer func() { _ = idx2.Close() }()
	off0, _, _, _, _, _ := idx2.Entry(0)
	off1, _, _, _, _, _ := idx2.Entry(1)
	if off0 != 0 || off1 != 4096 {
		t.Fatalf("unexpected offsets after compact: %d %d", off0, off1)
	}
}
