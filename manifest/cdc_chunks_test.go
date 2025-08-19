package manifest

import (
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
)

func TestIndexCDCChunks(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "man")
	idx, err := Create(manPath, "dev", 1024, 0, 4, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	data := []byte("hello world")
	digest := blake3.Sum256(data)
	if err := idx.Set(0, uint32(len(data)), FlagCDC, xxh3.Hash(data), digest); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	idx2, err := Open(manPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx2.Close()
	chunks := idx2.CDCChunks()
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Offset != 0 || chunks[0].Length != uint32(len(data)) || chunks[0].Digest != digest {
		t.Fatalf("chunk mismatch")
	}
}
