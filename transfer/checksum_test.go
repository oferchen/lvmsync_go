package transfer

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/zeebo/blake3"
)

func TestSHA256Checksum(t *testing.T) {
	data := []byte("verify sha256")
	want := sha256.Sum256(data)
	got := GetChecksumStrategy("sha256").Compute(data)
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("sha256 checksum mismatch: got %x, want %x", got, want)
	}
}

func TestBLAKE3Checksum(t *testing.T) {
	data := []byte("verify blake3")
	want := blake3.Sum256(data)
	got := GetChecksumStrategy("blake3").Compute(data)
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("blake3 checksum mismatch: got %x, want %x", got, want)
	}
}

func TestBLAKE3_512Checksum(t *testing.T) {
	data := []byte("verify blake3 512")
	want := blake3.Sum512(data)
	got := GetChecksumStrategy("blake3-512").Compute(data)
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("blake3-512 checksum mismatch: got %x, want %x", got, want)
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
