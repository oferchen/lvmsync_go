//go:build integration

package manifest

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
)

// TestGCCompactionSoak repeatedly compacts manifests to ensure deleted entries stay
// removed, remaining entries stay ordered, and file size remains stable.
// Expected runtime: ~2s.
func TestGCCompactionSoak(t *testing.T) {
	const (
		blockSize  = 4096
		chunks     = 64
		iterations = 50
	)
	dir := t.TempDir()
	path := filepath.Join(dir, "gc_compact.man")
	size := uint64(blockSize * chunks)
	idx, err := Create(path, "dev", size, 0, 0, 0, blockSize, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	live := make([]bool, chunks)
	buf := make([]byte, 128)
	for i := 0; i < chunks; i++ {
		rand.Read(buf)
		dig := blake3.Sum256(buf)
		if err := idx.Set(uint64(i*blockSize), uint32(len(buf)), 0, xxh3.Hash(buf), dig); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
		live[i] = true
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	baseSize := fi.Size()
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < iterations; i++ {
		i := i
		t.Run(fmt.Sprintf("iter-%d", i), func(t *testing.T) {
			idx, err := Open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			// delete roughly 10% of entries
			deleted := make([]int, 0, chunks/10)
			for j := 0; j < chunks/10; j++ {
				k := rng.Intn(chunks)
				off := uint64(k * blockSize)
				if err := idx.Set(off, 0, 0, 0, [32]byte{}); err != nil {
					t.Fatalf("delete %d: %v", k, err)
				}
				deleted = append(deleted, k)
				live[k] = false
			}
			if err := idx.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			if err := Compact(path); err != nil {
				t.Fatalf("compact: %v", err)
			}
			idx, err = Open(path)
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			defer idx.Close()
			var prev uint64
			for j := uint64(0); j < idx.ChunkCount(); j++ {
				off, length, _, _, _, err := idx.Entry(j)
				if err != nil {
					t.Fatalf("entry %d: %v", j, err)
				}
				idxNum := int(off / uint64(blockSize))
				if length == 0 {
					if live[idxNum] {
						t.Fatalf("live entry removed at offset %d", off)
					}
					continue
				}
				if !live[idxNum] {
					t.Fatalf("deleted entry present at offset %d", off)
				}
				if off < prev {
					t.Fatalf("entries out of order: %d < %d", off, prev)
				}
				prev = off
			}
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if fi.Size() > baseSize {
				t.Fatalf("manifest grew from %d to %d", baseSize, fi.Size())
			}
		})
	}
}
