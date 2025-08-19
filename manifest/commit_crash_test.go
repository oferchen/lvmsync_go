package manifest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// TestCommitCrashMidway ensures that if a manifest rewrite is interrupted
// before the new file is atomically swapped into place, the existing manifest
// remains valid.
func TestCommitCrashMidway(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest")
	idx, err := Create(path, "dev", 8192, 1, 0, 0, 4096, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var d [32]byte
	d[0] = 1
	if err := idx.Set(0, 4096, 0, 1, d); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read orig: %v", err)
	}

	idx2, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	type entry struct {
		off    uint64
		length uint32
		flags  uint32
		xxh    uint64
		digest [32]byte
	}
	entries := make([]entry, 0, idx2.hdr.ChunkCount)
	for i := uint64(0); i < idx2.hdr.ChunkCount; i++ {
		off, length, flags, xxh, dig, err := idx2.Entry(i)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if length == 0 {
			continue
		}
		entries = append(entries, entry{off, length, flags, xxh, dig})
	}
	hdr := idx2.hdr
	if err := idx2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
	deviceID := string(bytes.TrimRight(hdr.DeviceID[:], "\x00"))
	newIdx, err := Create(path, deviceID, hdr.SizeBytes, hdr.Epoch+1, hdr.Major, hdr.Minor, hdr.BlockSize, hdr.MinChunkSize, hdr.AvgChunkSize, hdr.MaxChunkSize, hdr.HybridFixedSize)
	if err != nil {
		t.Fatalf("Create new: %v", err)
	}
	for _, e := range entries {
		if err := newIdx.Set(e.off, e.length, e.flags, e.xxh, e.digest); err != nil {
			t.Fatalf("set new: %v", err)
		}
	}
	newIdx.hdr.MAC = headerMAC(&newIdx.hdr)
	newIdx.writeHeader()
	// Flush but do not Close, leaving tmp file and skipping fsyncRename.
	if err := unix.Msync(newIdx.data, unix.MS_SYNC); err != nil {
		t.Fatalf("msync: %v", err)
	}
	if err := newIdx.f.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := unix.Munmap(newIdx.data); err != nil {
		t.Fatalf("munmap: %v", err)
	}
	if err := newIdx.f.Close(); err != nil {
		t.Fatalf("close new: %v", err)
	}
	// Simulate crash: do not rename tmpPath.
	os.Remove(newIdx.tmpPath)

	// Reopen original manifest and ensure contents unchanged.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !bytes.Equal(orig, after) {
		t.Fatalf("manifest mutated during crash")
	}
	idx3, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := idx3.Close(); err != nil {
		t.Fatalf("close3: %v", err)
	}
}
