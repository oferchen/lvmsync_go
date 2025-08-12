package transfer

import (
	"crypto/sha256"
	"encoding/json"
)

// Chunk describes a block in the transfer with its BLAKE3 hash.
type Chunk struct {
	Hash   [32]byte `json:"hash"`
	Offset int64    `json:"offset"`
	Length int      `json:"length"`
}

// Manifest records all transferred chunks and the final SHA-256 hash.
// When resuming transfers, Bitmap carries a bitset of confirmed chunk indices
// to allow skipping already replicated data.
type Manifest struct {
	Chunks      []Chunk  `json:"chunks"`
	FinalSHA256 [32]byte `json:"final_sha256"`
	Bitmap      []byte   `json:"bitmap,omitempty"`
}

// Append adds a chunk entry to the manifest.
func (m *Manifest) Append(hash [32]byte, offset int64, length int) {
	m.Chunks = append(m.Chunks, Chunk{Hash: hash, Offset: offset, Length: length})
}

// Marshal encodes the manifest as JSON.
func (m *Manifest) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalManifest decodes a manifest from JSON.
func UnmarshalManifest(b []byte) (Manifest, error) {
	var m Manifest
	err := json.Unmarshal(b, &m)
	return m, err
}

// Verify recomputes the SHA-256 over the provided raw chunk data and compares
// it to the manifest's final digest.
func (m *Manifest) Verify(chunks [][]byte) bool {
	sha := sha256.New()
	for _, c := range chunks {
		sha.Write(c)
	}
	var sum [32]byte
	copy(sum[:], sha.Sum(nil))
	return sum == m.FinalSHA256
}
