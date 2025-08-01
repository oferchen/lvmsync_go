package transfer

import (
	"os"
	"testing"

	"lvmsync_go/config"
)

func TestChecksumDedupPersistence(t *testing.T) {
	f, err := os.CreateTemp("", "checksum_state")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	cfg := &config.Config{DedupStrategy: "checksum", DedupStateFile: f.Name()}
	d := NewDeduplicationStrategy(cfg)
	data := []byte("block")
	if !d.ShouldTransfer(0, data) {
		t.Fatalf("expected transfer on first check")
	}
	d.RecordTransfer(0, data)
	if err := d.SaveState(); err != nil {
		t.Fatal(err)
	}

	d2 := NewDeduplicationStrategy(cfg)
	if d2.ShouldTransfer(0, data) {
		t.Fatalf("expected block to be skipped after reload")
	}
}

func TestBloomFilterDedupPersistence(t *testing.T) {
	f, err := os.CreateTemp("", "bloom_state")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	cfg := &config.Config{DedupStrategy: "bloom", DedupStateFile: f.Name()}
	d := NewDeduplicationStrategy(cfg)
	data := []byte("block")
	if !d.ShouldTransfer(0, data) {
		t.Fatalf("expected transfer on first check")
	}
	d.RecordTransfer(0, data)
	if err := d.SaveState(); err != nil {
		t.Fatal(err)
	}

	d2 := NewDeduplicationStrategy(cfg)
	if d2.ShouldTransfer(0, data) {
		t.Fatalf("expected block to be skipped after reload")
	}
}
