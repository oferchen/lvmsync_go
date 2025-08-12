package dedup

import (
	"bytes"
	"testing"

	"github.com/zeebo/blake3"
)

// countingReader wraps a bytes.Reader and tracks the number of bytes read.
type countingReader struct {
	*bytes.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	c.read += n
	return n, err
}

func TestReassembleDuplicateChunks(t *testing.T) {
	chunk := []byte("hello world")
	hash := blake3.Sum256(chunk)
	man := Manifest{Chunks: []ManifestEntry{
		{Hash: hash, Offset: 0, Length: len(chunk)},
		{Hash: hash, Offset: int64(len(chunk)), Length: len(chunk)},
	}}
	cr := &countingReader{Reader: bytes.NewReader(chunk)}
	var out bytes.Buffer
	if err := Reassemble(man, cr, &out); err != nil {
		t.Fatalf("reassemble failed: %v", err)
	}
	expected := append(chunk, chunk...)
	if !bytes.Equal(out.Bytes(), expected) {
		t.Fatalf("unexpected output: %q", out.Bytes())
	}
	if cr.read != len(chunk) {
		t.Fatalf("expected to read %d bytes, read %d", len(chunk), cr.read)
	}
}
