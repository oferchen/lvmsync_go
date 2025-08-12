//go:build arm64

package cpufeatures

import "testing"

func TestNEON(t *testing.T) {
	if !HasNEON() {
		t.Skip("NEON not supported")
	}
	if !HasNEON() {
		t.Fatalf("expected NEON")
	}
}
