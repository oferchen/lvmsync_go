package transfer

import (
	"path/filepath"
	"testing"

	"github.com/bits-and-blooms/bloom/v3"
)

func TestBloomFilterDedupPersistence(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")

	d1 := &BloomFilterDedup{filter: bloom.NewWithEstimates(1000, 0.01), stateFile: stateFile}
	data := []byte("hello")

	if !d1.ShouldTransfer(0, data) {
		t.Fatalf("first transfer should be allowed")
	}
	d1.RecordTransfer(0, data)
	if err := d1.SaveState(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	d2 := &BloomFilterDedup{filter: bloom.NewWithEstimates(1000, 0.01), stateFile: stateFile}
	if err := d2.loadState(); err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if d2.ShouldTransfer(0, data) {
		t.Fatalf("expected bloom filter to persist state")
	}
}

func TestRollingHashDedupPersistence(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state")

	d1 := &RollingHashDedup{stateFile: stateFile, hashes: make(map[int64]uint64)}
	data := []byte("hello")

	if !d1.ShouldTransfer(0, data) {
		t.Fatalf("first transfer should be allowed")
	}
	d1.RecordTransfer(0, data)
	if err := d1.SaveState(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	d2 := &RollingHashDedup{stateFile: stateFile, hashes: make(map[int64]uint64)}
	if err := d2.loadState(); err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if d2.ShouldTransfer(0, data) {
		t.Fatalf("expected rolling hash dedup to persist state")
	}
}
