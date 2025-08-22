package hash

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/zeebo/blake3"

	cpufeatures "github.com/oferchen/lvmsync_go/internal/cpufeatures"
)

func TestSumXXH3(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"", 0x2d06800538d394c2},
		{"hello", 0x9555e8555c62dcfd},
	}
	for _, tc := range cases {
		if got := SumXXH3([]byte(tc.in)); got != tc.want {
			t.Fatalf("SumXXH3(%q) = %x want %x", tc.in, got, tc.want)
		}
	}
}

func TestSumBLAKE3(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262"},
		{"hello", "ea8f163db38682925e4491c5e58d4bb3506ef8c14eb78a86e908c5624a67200f"},
	}
	for _, tc := range cases {
		got := SumBLAKE3([]byte(tc.in))
		if hex.EncodeToString(got[:]) != tc.want {
			t.Fatalf("SumBLAKE3(%q) = %x want %s", tc.in, got, tc.want)
		}
	}
}

func BenchmarkSumXXH3(b *testing.B) {
	data := []byte("benchmark data for hashing")
	for i := 0; i < b.N; i++ {
		SumXXH3(data)
	}
}

func BenchmarkSumBLAKE3(b *testing.B) {
	data := []byte("benchmark data for hashing")
	for i := 0; i < b.N; i++ {
		SumBLAKE3(data)
	}
}

func TestHasSIMD(t *testing.T) {
	if HasSIMD() != cpufeatures.HasSIMD() {
		t.Fatalf("HasSIMD mismatch")
	}
}

func TestBlake3Hasher_Unkeyed(t *testing.T) {
	h, err := NewBlake3Hasher(nil)
	if err != nil {
		t.Fatalf("NewBlake3Hasher: %v", err)
	}
	input1 := []byte("hello")
	if n, err := h.Write(input1); err != nil || n != len(input1) {
		t.Fatalf("Write(%q) = %d, %v", input1, n, err)
	}
	got1 := h.Sum256()
	want1 := blake3.Sum256(input1)
	if got1 != want1 {
		t.Fatalf("Sum256(%q) = %x want %x", input1, got1, want1)
	}
	input2 := []byte("world")
	if _, err := h.Write(input2); err != nil {
		t.Fatalf("Write(%q) err: %v", input2, err)
	}
	got2 := h.Sum256()
	want2 := blake3.Sum256(input2)
	if got2 != want2 {
		t.Fatalf("Sum256 reset failed: got %x want %x", got2, want2)
	}
}

func TestBlake3Hasher_Keyed(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	h, err := NewBlake3Hasher(key)
	if err != nil {
		t.Fatalf("NewBlake3Hasher keyed: %v", err)
	}
	data := []byte("data")
	if _, err := h.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := h.Sum256()
	k, err := blake3.NewKeyed(key)
	if err != nil {
		t.Fatalf("NewKeyed: %v", err)
	}
	k.Write(data)
	want := k.Sum(nil)
	if !bytes.Equal(got[:], want) {
		t.Fatalf("keyed digest mismatch: got %x want %x", got, want)
	}
	unkeyed := blake3.Sum256(data)
	if bytes.Equal(got[:], unkeyed[:]) {
		t.Fatalf("keyed digest should differ from unkeyed")
	}
}

func TestBlake3Hasher_Reset(t *testing.T) {
	h, err := NewBlake3Hasher(nil)
	if err != nil {
		t.Fatalf("NewBlake3Hasher: %v", err)
	}
	if _, err := h.Write([]byte("foo")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	h.Reset()
	if _, err := h.Write([]byte("bar")); err != nil {
		t.Fatalf("Write after reset: %v", err)
	}
	got := h.Sum256()
	want := blake3.Sum256([]byte("bar"))
	if got != want {
		t.Fatalf("Reset failed: got %x want %x", got, want)
	}
}

func TestBlake3Hasher_Digest(t *testing.T) {
	h, err := NewBlake3Hasher(nil)
	if err != nil {
		t.Fatalf("NewBlake3Hasher: %v", err)
	}
	d := h.Digest()
	d.Write([]byte("first"))
	want1 := blake3.Sum256([]byte("first"))
	if got := h.Sum256(); got != want1 {
		t.Fatalf("Digest write not reflected: got %x want %x", got, want1)
	}
	d.Write([]byte("second"))
	want2 := blake3.Sum256([]byte("second"))
	if got := h.Sum256(); got != want2 {
		t.Fatalf("Digest not reset: got %x want %x", got, want2)
	}
}
