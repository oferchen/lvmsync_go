package lvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateSnapshotWithEscalationNonRoot(t *testing.T) {
	tmpDir := t.TempDir()
	argsFile := filepath.Join(tmpDir, "args")
	script := filepath.Join(tmpDir, "esc.sh")
	content := fmt.Sprintf("#!/bin/sh\nprintf '%%s %%s' \"$1\" \"$2\" > %s\n", argsFile)
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	t.Cleanup(func() {
		SetEscalationCommand("")
	})

	SetEscalationCommand(script)
	restore := SetBackend(nil)
	t.Cleanup(restore)

	orig := checkPrivs
	checkPrivs = func() error {
		if GetEscalationCommand() == "" {
			return fmt.Errorf("privileges required")
		}
		return nil
	}
	t.Cleanup(func() { checkPrivs = orig })

	if err := CreateSnapshot(context.Background(), "/dev/vg0/origin", "snap", "1G"); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}
	if string(data) != "lvm lvcreate" {
		t.Fatalf("unexpected args %q", string(data))
	}
}

func TestCleanupRemovesWrapper(t *testing.T) {
	t.Cleanup(func() { SetEscalationCommand("") })

	SetEscalationCommand("/bin/true")
	restore := SetBackend(nil)
	t.Cleanup(restore)

	b, ok := backend.(*lvm2Backend)
	if !ok {
		t.Fatalf("unexpected backend type %T", backend)
	}
	if b.wrapperPath == "" {
		t.Fatal("wrapper path not set")
	}

	path := b.wrapperPath
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("wrapper file not created: %v", err)
	}

	Cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("wrapper file still exists after cleanup: %v", err)
	}
}
