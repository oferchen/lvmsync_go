package manifest

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
)

// TestCompactionRemovesDeleted ensures deleted entries are removed and persisted.
func TestCompactionRemovesDeleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compaction.man")
	idx, err := Create(path, "dev", 8192, 0, 0, 0, 4096, 0, 0, 0, 0)
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
	// Delete first entry by clearing length.
	off := entryOffset(0)
	binary.LittleEndian.PutUint32(idx.data[int(off)+8:int(off)+12], 0)
	idx.buildTables()
	if idx.Match(0, uint32(len(a)), 0, xxh3.Hash(a), func() [32]byte { return blake3.Sum256(a) }) {
		t.Fatalf("deleted entry still matches")
	}
	if !idx.Match(4096, uint32(len(b)), 0, xxh3.Hash(b), func() [32]byte { return blake3.Sum256(b) }) {
		t.Fatalf("expected remaining entry to match")
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Reopen to ensure deletion persisted.
	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if idx2.Match(0, uint32(len(a)), 0, xxh3.Hash(a), func() [32]byte { return blake3.Sum256(a) }) {
		t.Fatalf("deleted entry persisted")
	}
	if !idx2.Match(4096, uint32(len(b)), 0, xxh3.Hash(b), func() [32]byte { return blake3.Sum256(b) }) {
		t.Fatalf("remaining entry missing after reopen")
	}
	if err := idx2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}
