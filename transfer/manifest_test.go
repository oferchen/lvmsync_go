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

func TestManifestVerify(t *testing.T) {
	data1 := []byte("foo")
	data2 := []byte("bar")
	var m Manifest
	h1 := blake3.Sum256(data1)
	m.Append(h1, 0, len(data1))
	h2 := blake3.Sum256(data2)
	m.Append(h2, int64(len(data1)), len(data2))
	sha := sha256.New()
	sha.Write(data1)
	sha.Write(data2)
	copy(m.FinalSHA256[:], sha.Sum(nil))
	if !m.Verify([][]byte{data1, data2}) {
		t.Fatalf("verify failed")
	}
	data1[0]++
	if m.Verify([][]byte{data1, data2}) {
		t.Fatalf("verify should fail")
	}
}
