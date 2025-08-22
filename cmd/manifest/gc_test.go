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

// entrySize is the size of a manifest entry in bytes.
const entrySize = 56

func TestRunGCRemovesDuplicates(t *testing.T) {
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
	defer f.Close()
	buf := make([]byte, entrySize)
	if _, err := f.ReadAt(buf, int64(manifestpkg.HeaderSize)); err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if _, err := f.WriteAt(buf, int64(manifestpkg.HeaderSize+entrySize)); err != nil {
		t.Fatalf("write duplicate: %v", err)
	}
	f.Close()
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if err := Run(cfg, []string{"gc", path}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	idx2, err := manifestpkg.Open(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer idx2.Close()
	_, l0, _, _, _, _ := idx2.Entry(0)
	_, l1, _, _, _, _ := idx2.Entry(1)
	if l0 == 0 || l1 != 0 {
		t.Fatalf("unexpected lengths after gc: %d %d", l0, l1)
	}
}
