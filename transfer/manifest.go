package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"

	"github.com/zeebo/blake3"
)

// Chunk describes a block in the transfer with its BLAKE3 hash.
type Chunk struct {
	Hash   [32]byte `json:"hash"`
	Offset int64    `json:"offset"`
	Length int      `json:"length"`
}

// Manifest records all transferred chunks and the final device digest.
type Manifest struct {
	Chunks      []Chunk `json:"chunks"`
	FinalDigest []byte  `json:"final_digest"`
	DigestAlgo  string  `json:"digest_algo"`
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

// MissingIndices returns indices from m that are absent in peer.
func (m *Manifest) MissingIndices(peer Manifest) []int {
	present := make(map[[32]byte]struct{}, len(peer.Chunks))
	for _, c := range peer.Chunks {
		present[c.Hash] = struct{}{}
	}
	var missing []int
	for i, c := range m.Chunks {
		if _, ok := present[c.Hash]; !ok {
			missing = append(missing, i)
		}
	}
	return missing
}

func newDigestHasher(algo string) hash.Hash {
	switch strings.ToLower(algo) {
	case "blake3", "blake3-256":
		return blake3.New()
	default:
		return sha256.New()
	}
}

// Verify recomputes the digest over the provided raw chunk data and compares
// it to the manifest's final digest.
func (m *Manifest) Verify(chunks [][]byte) bool {
	h := newDigestHasher(m.DigestAlgo)
	for _, c := range chunks {
		h.Write(c)
	}
	return bytes.Equal(h.Sum(nil), m.FinalDigest)
}

// VerifyDevice computes the digest of the given device or file and compares it
// against the manifest's final digest.
func (m *Manifest) VerifyDevice(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open device for digest: %w", err)
	}
	defer f.Close()

	h := newDigestHasher(m.DigestAlgo)
	buf := make([]byte, 1<<20)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read device: %w", err)
		}
	}
	if !bytes.Equal(h.Sum(nil), m.FinalDigest) {
		return fmt.Errorf("final checksum mismatch")
	}
	return nil
}
