package transfer

import (
	"bytes"
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
	m.FinalDigest = sha[:]
	m.DigestAlgo = "sha256"

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
	if !bytes.Equal(m2.FinalDigest, sha[:]) {
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
	m.FinalDigest = sha.Sum(nil)
	m.DigestAlgo = "sha256"
	if !m.Verify([][]byte{data1, data2}) {
		t.Fatalf("verify failed")
	}
	data1[0]++
	if m.Verify([][]byte{data1, data2}) {
		t.Fatalf("verify should fail")
	}
}

func TestMissingIndices(t *testing.T) {
	var a, b Manifest
	d1 := []byte("foo")
	h1 := blake3.Sum256(d1)
	a.Append(h1, 0, len(d1))
	a.FinalDigest = []byte{1}
	a.DigestAlgo = "sha256"

	d2 := []byte("bar")
	h2 := blake3.Sum256(d2)
	a.Append(h2, int64(len(d1)), len(d2))

	b.Append(h1, 0, len(d1))
	b.FinalDigest = []byte{1}
	b.DigestAlgo = "sha256"

	missing := a.MissingIndices(b)
	if len(missing) != 1 || missing[0] != 1 {
		t.Fatalf("unexpected missing indices %v", missing)
	}
}
