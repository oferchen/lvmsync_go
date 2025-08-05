package lvm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadUintAttrMissingAndMalformed(t *testing.T) {
	tmpDir := t.TempDir()

	if _, err := readUintAttr(tmpDir, "missing"); err == nil {
		t.Errorf("expected error for missing file")
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "bad"), []byte("abc"), 0644); err != nil {
		t.Fatalf("failed to write bad file: %v", err)
	}
	if _, err := readUintAttr(tmpDir, "bad"); err == nil {
		t.Errorf("expected error for malformed content")
	}
}

func TestReadBoolAttrMissingAndMalformed(t *testing.T) {
	tmpDir := t.TempDir()

	if _, err := readBoolAttr(tmpDir, "missing"); err == nil {
		t.Errorf("expected error for missing file")
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "bad"), []byte("2"), 0644); err != nil {
		t.Fatalf("failed to write bad file: %v", err)
	}
	if _, err := readBoolAttr(tmpDir, "bad"); err == nil {
		t.Errorf("expected error for malformed content")
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "badstr"), []byte("abc"), 0644); err != nil {
		t.Fatalf("failed to write bad string file: %v", err)
	}
	if _, err := readBoolAttr(tmpDir, "badstr"); err == nil {
		t.Errorf("expected error for non-numeric content")
	}
}
