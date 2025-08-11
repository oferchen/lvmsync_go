package hash

import "testing"

func TestBlake3Determinism(t *testing.T) {
	h := NewBlake3()
	a := h.Sum([]byte("hello"))
	b := h.Sum([]byte("hello"))
	if a != b {
		t.Fatalf("inconsistent")
	}
}

func TestSHA256Determinism(t *testing.T) {
	h := NewSHA256()
	a := h.Sum([]byte("world"))
	b := h.Sum([]byte("world"))
	if a != b {
		t.Fatalf("inconsistent")
	}
}

func TestDetect(t *testing.T) {
	d := Detect()
	if !d.AVX2 && !d.SSE41 && !d.AVX512 { // ensure at least detection ran
		t.Log("no SIMD, but detect executed")
	}
}
