package hash

import (
	"encoding/hex"
	"testing"

	cpufeatures "lvmsync_go/internal/cpufeatures"
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
