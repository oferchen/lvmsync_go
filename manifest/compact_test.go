// Package manifest tests manifest compaction.
package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
)

func TestCompactPreservesCardinalityAndOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compact.man")
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

	off0 := int(entryOffset(0))
	off1 := int(entryOffset(1))
	tmp := make([]byte, entrySize)
	copy(tmp, idx.data[off0:off0+entrySize])
	copy(idx.data[off0:off0+entrySize], idx.data[off1:off1+entrySize])
	copy(idx.data[off1:off1+entrySize], tmp)

	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	offsets := []uint64{}
	count := 0
	for i := uint64(0); i < idx2.hdr.ChunkCount; i++ {
		off, length, _, _, _, err := idx2.Entry(i)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if length == 0 {
			continue
		}
		offsets = append(offsets, off)
		count++
	}
	if count != 2 {
		t.Fatalf("unexpected count before compact: %d", count)
	}
	if len(offsets) != 2 || offsets[0] <= offsets[1] {
		t.Fatalf("expected offsets out of order before compact: %v", offsets)
	}
	if err := idx2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}

	if err := Compact(path); err != nil {
		t.Fatalf("compact: %v", err)
	}

	idx3, err := Open(path)
	if err != nil {
		t.Fatalf("open3: %v", err)
	}
	offsets = offsets[:0]
	count = 0
	for i := uint64(0); i < idx3.hdr.ChunkCount; i++ {
		off, length, _, _, _, err := idx3.Entry(i)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if length == 0 {
			continue
		}
		offsets = append(offsets, off)
		count++
	}
	if count != 2 {
		t.Fatalf("unexpected count after compact: %d", count)
	}
	if len(offsets) != 2 || offsets[0] >= offsets[1] {
		t.Fatalf("expected offsets ordered after compact: %v", offsets)
	}
	if err := idx3.Close(); err != nil {
		t.Fatalf("close3: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}
