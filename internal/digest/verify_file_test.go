package digest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name, data string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return p
}

func TestVerifyFilesMatch(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a", "same data")
	b := writeTemp(t, dir, "b", "same data")
	match, _, _, err := VerifyFiles(a, b, SHA256, false)
	if err != nil {
		t.Fatalf("VerifyFiles: %v", err)
	}
	if !match {
		t.Fatalf("expected digests to match")
	}
}

func TestVerifyFilesMismatch(t *testing.T) {
	dir := t.TempDir()
	a := writeTemp(t, dir, "a", "foo")
	b := writeTemp(t, dir, "b", "bar")
	match, d1, d2, err := VerifyFiles(a, b, BLAKE3, false)
	if err != nil {
		t.Fatalf("VerifyFiles: %v", err)
	}
	if match {
		t.Fatalf("expected mismatch")
	}
	if d1 == d2 {
		t.Fatalf("expected different digests")
	}
}
