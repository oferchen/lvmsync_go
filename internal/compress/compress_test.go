package compress

import (
	"bytes"
	"testing"
)

func TestLZ4RoundTrip(t *testing.T) {
	c := NewLZ4()
	in := bytes.Repeat([]byte{1}, 1024)
	out, used, err := c.Compress(in)
	if err != nil || !used {
		t.Fatalf("compress: %v", err)
	}
	dec, err := c.Decompress(out)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(in, dec) {
		t.Fatalf("mismatch")
	}
}

func TestZstdRoundTrip(t *testing.T) {
	c := NewZstd()
	in := bytes.Repeat([]byte{2}, 1024)
	out, used, err := c.Compress(in)
	if err != nil || !used {
		t.Fatalf("compress: %v", err)
	}
	dec, err := c.Decompress(out)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(in, dec) {
		t.Fatalf("mismatch")
	}
}

func TestAuto(t *testing.T) {
	c := NewAuto()
	in := bytes.Repeat([]byte{3}, 1024)
	out, used, err := c.Compress(in)
	if err != nil || !used {
		t.Fatalf("compress: %v", err)
	}
	dec, err := c.Decompress(out)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(in, dec) {
		t.Fatalf("mismatch")
	}
}
