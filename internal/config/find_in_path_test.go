package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindInPath(t *testing.T) {
	t.Run("absoluteExecutable", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "tool")
		if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write exec: %v", err)
		}
		got, err := findInPath(exe)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != exe {
			t.Fatalf("expected %s, got %s", exe, got)
		}
	})

	t.Run("relativeInPath", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "tool")
		if err := os.WriteFile(exe, []byte(""), 0o755); err != nil {
			t.Fatalf("write exec: %v", err)
		}
		t.Setenv("PATH", dir)
		got, err := findInPath("tool")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != exe {
			t.Fatalf("expected %s, got %s", exe, got)
		}
	})

	t.Run("nonExecutable", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "file")
		if err := os.WriteFile(f, []byte(""), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		if _, err := findInPath(f); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PATH", dir)
		if _, err := findInPath("does-not-exist"); err == nil {
			t.Fatalf("expected error")
		}
	})
}
