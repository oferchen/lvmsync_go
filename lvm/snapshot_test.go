package lvm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	SetEscalationCommand("")
}

func TestCreateAndRemoveSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock lvcreate script
	lvcreateScript := filepath.Join(tmpDir, "lvcreate")
	lvcreateContent := "#!/bin/sh\necho \"$@\" > \"" + filepath.Join(tmpDir, "lvcreate_args") + "\"\n"
	if err := os.WriteFile(lvcreateScript, []byte(lvcreateContent), 0755); err != nil {
		t.Fatalf("failed to write lvcreate script: %v", err)
	}

	// Create mock lvremove script
	lvremoveScript := filepath.Join(tmpDir, "lvremove")
	lvremoveContent := "#!/bin/sh\necho \"$@\" > \"" + filepath.Join(tmpDir, "lvremove_args") + "\"\n"
	if err := os.WriteFile(lvremoveScript, []byte(lvremoveContent), 0755); err != nil {
		t.Fatalf("failed to write lvremove script: %v", err)
	}

	// Prepend tmpDir to PATH so our scripts are used
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)

	lvPath := "/dev/vg0/origin"
	snapName := "snap"
	size := "1G"

	if err := CreateSnapshot(lvPath, snapName, size); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	argsData, err := os.ReadFile(filepath.Join(tmpDir, "lvcreate_args"))
	if err != nil {
		t.Fatalf("failed to read lvcreate args: %v", err)
	}
	got := strings.TrimSpace(string(argsData))
	expected := "-s -n " + snapName + " -L " + size + " " + lvPath
	if got != expected {
		t.Fatalf("lvcreate args = %q, want %q", got, expected)
	}

	snapPath := "/dev/vg0/" + snapName
	if err := RemoveSnapshot(snapPath); err != nil {
		t.Fatalf("RemoveSnapshot failed: %v", err)
	}

	rmArgs, err := os.ReadFile(filepath.Join(tmpDir, "lvremove_args"))
	if err != nil {
		t.Fatalf("failed to read lvremove args: %v", err)
	}
	got = strings.TrimSpace(string(rmArgs))
	expected = "-f " + snapPath
	if got != expected {
		t.Fatalf("lvremove args = %q, want %q", got, expected)
	}
}
