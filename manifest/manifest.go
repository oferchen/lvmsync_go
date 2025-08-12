package manifest

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/bits-and-blooms/bitset"
)

// Version is the manifest format version.
const Version uint32 = 1

// Manifest tracks chunk completion and hashes for a logical volume.
type Manifest struct {
	LVUUID       string         `json:"lv_uuid"`
	SizeBytes    uint64         `json:"size_bytes"`
	ChunkSize    uint64         `json:"chunk_size"`
	TotalChunks  uint64         `json:"total_chunks"`
	Bitmap       *bitset.BitSet `json:"bitmap"`
	PerChunkHash [][]byte       `json:"per_chunk_hash"`
	Version      uint32         `json:"version"`
}

// Save writes the manifest to the given path in JSON format.
func (m *Manifest) Save(path string) error {
	if m == nil {
		return errors.New("nil manifest")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads a manifest from the given path.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Bitmap == nil {
		m.Bitmap = bitset.New(0)
	}
	return &m, nil
}
