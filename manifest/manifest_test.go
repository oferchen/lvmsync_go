package manifest

import (
	"os"
	"testing"

	"github.com/bits-and-blooms/bitset"
	"github.com/zeebo/blake3"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	m := &Manifest{
		LVUUID:      "1234-5678",
		SizeBytes:   1024,
		ChunkSize:   256,
		TotalChunks: 4,
		Bitmap:      bitset.New(4),
		Version:     Version,
	}
	h0 := blake3.Sum256([]byte("chunk0"))
	h2 := blake3.Sum256([]byte("chunk2"))
	m.Bitmap.Set(0)
	m.Bitmap.Set(2)
	m.PerChunkHash = [][]byte{h0[:], nil, h2[:]}

	f, err := os.CreateTemp(t.TempDir(), "manifest-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	if err := m.Save(f.Name()); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(f.Name())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.Bitmap.Equal(m.Bitmap) {
		t.Fatalf("bitmap mismatch")
	}
	if len(loaded.PerChunkHash) != len(m.PerChunkHash) {
		t.Fatalf("per chunk hash length mismatch")
	}
	if string(loaded.PerChunkHash[0]) != string(h0[:]) || string(loaded.PerChunkHash[2]) != string(h2[:]) {
		t.Fatalf("hashes mismatch")
	}
	if loaded.LVUUID != m.LVUUID || loaded.SizeBytes != m.SizeBytes || loaded.ChunkSize != m.ChunkSize || loaded.TotalChunks != m.TotalChunks || loaded.Version != m.Version {
		t.Fatalf("fields mismatch")
	}
}

func TestLoadPartialManifest(t *testing.T) {
	m := &Manifest{
		LVUUID:      "partial",
		SizeBytes:   2048,
		ChunkSize:   512,
		TotalChunks: 4,
		Bitmap:      bitset.New(4),
		Version:     Version,
	}
	h0 := blake3.Sum256([]byte("chunk0"))
	m.Bitmap.Set(0)
	m.PerChunkHash = [][]byte{h0[:]}

	f, err := os.CreateTemp(t.TempDir(), "manifest-partial-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	if err := m.Save(f.Name()); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(f.Name())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.PerChunkHash) != 1 {
		t.Fatalf("expected 1 hash, got %d", len(loaded.PerChunkHash))
	}
	if !loaded.Bitmap.Test(0) {
		t.Fatalf("expected bit 0 set")
	}
}

func TestLoadCorruptManifest(t *testing.T) {
	m := &Manifest{LVUUID: "bad", Bitmap: bitset.New(1), Version: Version}
	f, err := os.CreateTemp(t.TempDir(), "manifest-corrupt-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	if err := m.Save(f.Name()); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := os.WriteFile(f.Name(), []byte("not json"), 0o600); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := Load(f.Name()); err == nil {
		t.Fatalf("expected error on corrupt manifest")
	}
}
