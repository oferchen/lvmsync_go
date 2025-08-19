package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunGeneratesDoc(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	path := filepath.Join(tmp, "docs", "config_env.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected doc at %s: %v", path, err)
	}
}

func TestRunMissingGoMod(t *testing.T) {
	tmp := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := run(); err == nil {
		t.Fatal("expected error")
	}
}
