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

func TestUnsupportedAlgorithmDefaultsToSHA256(t *testing.T) {
	data := []byte("verify default")
	want := sha256.Sum256(data)
	cases := []string{"unsupported", "blake3-512"}
	for _, alg := range cases {
		got := GetChecksumStrategy(alg).Compute(data)
		if !bytes.Equal(got, want[:]) {
			t.Fatalf("%s checksum mismatch: got %x, want %x", alg, got, want)
		}
	}
}

func TestAutoChecksumSelects(t *testing.T) {
	orig := detectChecksumAlgorithm
	defer func() { detectChecksumAlgorithm = orig }()

	detectChecksumAlgorithm = func() string { return "blake3" }
	if _, ok := GetChecksumStrategy("auto").(*BLAKE3Checksum); !ok {
		t.Fatalf("expected blake3 strategy")
	}
	detectChecksumAlgorithm = func() string { return "sha256" }
	if _, ok := GetChecksumStrategy("auto").(*SHA256Checksum); !ok {
		t.Fatalf("expected sha256 strategy")
	}
}
