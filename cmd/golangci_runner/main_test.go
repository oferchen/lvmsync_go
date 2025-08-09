package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureLinterFound(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)
	path := filepath.Join(binDir, "golangci-lint")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake linter: %v", err)
	}
	called := false
	oldInstall := installLinter
	t.Cleanup(func() { installLinter = oldInstall })
	installLinter = func() error {
		called = true
		return nil
	}

	p, err := ensureLinter()
	if err != nil {
		t.Fatalf("ensureLinter: %v", err)
	}
	if called {
		t.Fatalf("installLinter should not be called")
	}
	if p != path {
		t.Fatalf("expected %s, got %s", path, p)
	}
}

func TestEnsureLinterInstalls(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)

	installCalled := false
	oldInstall := installLinter
	t.Cleanup(func() { installLinter = oldInstall })
	installLinter = func() error {
		installCalled = true
		path := filepath.Join(binDir, "golangci-lint")
		return os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)
	}

	p, err := ensureLinter()
	if err != nil {
		t.Fatalf("ensureLinter: %v", err)
	}
	if !installCalled {
		t.Fatalf("installLinter was not called")
	}
	expected := filepath.Join(binDir, "golangci-lint")
	if p != expected {
		t.Fatalf("expected %s, got %s", expected, p)
	}
}
