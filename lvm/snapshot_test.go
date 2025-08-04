package lvm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func init() {
	SetEscalationCommand("")
}

func TestCreateAndRemoveSnapshot(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })

	var cmds []string
	origRun := runLVMCommand
	runLVMCommand = func(name string, args ...string) ([]byte, error) {
		cmds = append(cmds, strings.Join(append([]string{name}, args...), " "))
		return []byte(""), nil
	}
	t.Cleanup(func() { runLVMCommand = origRun })

	lvPath := "/dev/vg0/origin"
	snapName := "snap"
	size := "1G"

	if err := CreateSnapshot(lvPath, snapName, size); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	snapPath := "/dev/vg0/" + snapName
	if err := RemoveSnapshot(snapPath); err != nil {
		t.Fatalf("RemoveSnapshot failed: %v", err)
	}

	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if cmds[0] != fmt.Sprintf("lvcreate -s -n %s -L %s %s", snapName, size, lvPath) {
		t.Fatalf("lvcreate args = %q", cmds[0])
	}
	if cmds[1] != fmt.Sprintf("lvremove -f %s", snapPath) {
		t.Fatalf("lvremove args = %q", cmds[1])
	}
}

func TestCreateSnapshotPrivilegeError(t *testing.T) {
	orig := checkPrivs
	errPriv := errors.New("privileges required")
	checkPrivs = func() error { return errPriv }
	t.Cleanup(func() { checkPrivs = orig })

	err := CreateSnapshot("/dev/vg0/origin", "snap", "1G")
	if !errors.Is(err, errPriv) {
		t.Fatalf("expected privilege error, got %v", err)
	}
}
