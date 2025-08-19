package root

import (
	"os"
	"testing"
)

func TestConfigureStdoutRequiresConfirmation(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"cmd", "--stdout", "/src"}

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		r.Close()
		w.Close()
	}()

	if _, _, _, err := ConfigureWithEscalator(stubEscalator{}); err == nil {
		t.Fatalf("expected confirmation error")
	}
}

func TestConfigureStdoutConfirmed(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"cmd", "--stdout", "--yes-i-know", "/src"}

	origStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		r.Close()
		w.Close()
	}()

	if _, _, _, err := ConfigureWithEscalator(stubEscalator{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
