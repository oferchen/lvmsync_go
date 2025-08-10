package transfer

import (
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/zeebo/blake3"
)

func TestManifestRoundTrip(t *testing.T) {
	var m Manifest
	data := []byte("hello world")
	sum := blake3.Sum256(data)
	m.Append(sum, 0, len(data))
	sha := sha256.Sum256(data)
	m.FinalSHA256 = sha

	b, err := m.Marshal()
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	var m2 Manifest
	if err := json.Unmarshal(b, &m2); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(m2.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(m2.Chunks))
	}
	if m2.Chunks[0].Offset != 0 || m2.Chunks[0].Length != len(data) {
		t.Fatalf("unexpected chunk values: %#v", m2.Chunks[0])
	}
	if m2.FinalSHA256 != sha {
		t.Fatalf("final SHA mismatch")
	}
}
