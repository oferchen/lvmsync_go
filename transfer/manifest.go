package transfer

import "encoding/json"

// Chunk describes a block in the transfer with its BLAKE3 hash.
type Chunk struct {
	Hash   [32]byte `json:"hash"`
	Offset int64    `json:"offset"`
	Length int      `json:"length"`
}

// Manifest records all transferred chunks and the final SHA-256 hash.
type Manifest struct {
	Chunks      []Chunk  `json:"chunks"`
	FinalSHA256 [32]byte `json:"final_sha256"`
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
