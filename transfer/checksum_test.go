package transfer

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestSHA256Checksum(t *testing.T) {
	data := []byte("verify sha256")
	want := sha256.Sum256(data)
	got := GetChecksumStrategy("sha256").Compute(data)
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("sha256 checksum mismatch: got %x, want %x", got, want)
	}
}

func TestUnsupportedAlgorithmDefaultsToSHA256(t *testing.T) {
	data := []byte("verify default")
	want := sha256.Sum256(data)
	got := GetChecksumStrategy("md5").Compute(data)
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("default checksum mismatch: got %x, want %x", got, want)
	}
}
