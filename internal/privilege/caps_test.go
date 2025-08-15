//go:build linux

package privilege

import "testing"

func TestCheckCaps(t *testing.T) {
	prev := HasCaps
	defer func() { HasCaps = prev }()

	HasCaps = func() bool { return true }
	if err := checkCaps(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	HasCaps = func() bool { return false }
	if err := checkCaps(); err == nil {
		t.Fatalf("expected error when capabilities missing")
	}
}
