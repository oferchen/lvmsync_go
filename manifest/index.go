package manifest

import (
	"bytes"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// openTemp creates a temporary manifest file alongside the target path and mmaps it.
func openTemp(path string, total int64) (f *os.File, data []byte, tmpPath string, err error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	f, err = os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return nil, nil, "", err
	}
	tmpPath = f.Name()
	if err = f.Truncate(total); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, nil, "", err
	}
	data, err = unix.Mmap(int(f.Fd()), 0, int(total), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, nil, "", err
	}
	return f, data, tmpPath, nil
}

// fsyncRename atomically replaces oldPath with tmpPath, ensuring contents are flushed.
func fsyncRename(tmpPath, finalPath string) error {
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(finalPath))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// GC rewrites the manifest at path, omitting orphaned or duplicate entries.
func GC(path string, opts ...IndexOption) error {
	idx, err := Open(path, opts...)
	if err != nil {
		return err
	}
	// Collect live entries before closing the existing index.
	type entry struct {
		off    uint64
		length uint32
		flags  uint32
		xxh    uint64
		digest [32]byte
	}
	seen := make(map[uint64]struct{})
	entries := make([]entry, 0, idx.hdr.ChunkCount)
	for i := uint64(0); i < idx.hdr.ChunkCount; i++ {
		off, length, flags, xxh, dig, err := idx.Entry(i)
		if err != nil {
			idx.Close()
			return err
		}
		if length == 0 {
			continue
		}
		if _, ok := seen[off]; ok {
			continue
		}
		seen[off] = struct{}{}
		entries = append(entries, entry{off, length, flags, xxh, dig})
	}
	hdr := idx.hdr
	if err := idx.Close(); err != nil {
		return err
	}

	deviceID := string(bytes.TrimRight(hdr.DeviceID[:], "\x00"))
	newIdx, err := Create(path, deviceID, hdr.SizeBytes, hdr.Epoch, hdr.Major, hdr.Minor, hdr.BlockSize, hdr.MinChunkSize, hdr.AvgChunkSize, hdr.MaxChunkSize, hdr.HybridFixedSize, opts...)
	if err != nil {
		return err
	}
	copy(newIdx.hdr.FirstBlockDigest[:], hdr.FirstBlockDigest[:])
	for _, e := range entries {
		if err := newIdx.Set(e.off, e.length, e.flags, e.xxh, e.digest); err != nil {
			newIdx.Close()
			return err
		}
	}
	newIdx.hdr.MAC = headerMAC(&newIdx.hdr)
	newIdx.writeHeader()
	return newIdx.Close()
}

// Compact rewrites the manifest at path without dropping entries.
// A temporary file is written and fsync'd before atomically replacing the original.
func Compact(path string, opts ...IndexOption) error {
	idx, err := Open(path, opts...)
	if err != nil {
		return err
	}
	type entry struct {
		off    uint64
		length uint32
		flags  uint32
		xxh    uint64
		digest [32]byte
	}
	entries := make([]entry, 0, idx.hdr.ChunkCount)
	for i := uint64(0); i < idx.hdr.ChunkCount; i++ {
		off, length, flags, xxh, dig, err := idx.Entry(i)
		if err != nil {
			idx.Close()
			return err
		}
		entries = append(entries, entry{off, length, flags, xxh, dig})
	}
	hdr := idx.hdr
	if err := idx.Close(); err != nil {
		return err
	}

	deviceID := string(bytes.TrimRight(hdr.DeviceID[:], "\x00"))
	newIdx, err := Create(path, deviceID, hdr.SizeBytes, hdr.Epoch, hdr.Major, hdr.Minor, hdr.BlockSize, hdr.MinChunkSize, hdr.AvgChunkSize, hdr.MaxChunkSize, hdr.HybridFixedSize, opts...)
	if err != nil {
		return err
	}
	copy(newIdx.hdr.FirstBlockDigest[:], hdr.FirstBlockDigest[:])
	for _, e := range entries {
		if err := newIdx.Set(e.off, e.length, e.flags, e.xxh, e.digest); err != nil {
			newIdx.Close()
			return err
		}
	}
	newIdx.hdr.MAC = headerMAC(&newIdx.hdr)
	newIdx.writeHeader()
	return newIdx.Close()
}
