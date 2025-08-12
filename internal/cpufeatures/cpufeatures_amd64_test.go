//go:build amd64

package cpufeatures

import "testing"

func TestAESNI(t *testing.T) {
	if !HasAESNI() {
		t.Skip("AES-NI not supported")
	}
	if !HasAESNI() {
		t.Fatalf("expected AES-NI")
	}
}

func TestAVX2(t *testing.T) {
	if !HasAVX2() {
		t.Skip("AVX2 not supported")
	}
	if !HasAVX2() {
		t.Fatalf("expected AVX2")
	}
}

func TestAVX512(t *testing.T) {
	if !HasAVX512() {
		t.Skip("AVX-512 not supported")
	}
	if !HasAVX512() {
		t.Fatalf("expected AVX-512")
	}
}
