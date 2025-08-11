package privilege

import (
	"errors"
	"os/exec"
	"testing"
)

// fakeLookPath stubs exec.LookPath.
func fakeLookPath(err error) func(string) (string, error) {
	return func(string) (string, error) { return "/usr/bin/sudo", err }
}

func TestEnsureWithCaps(t *testing.T) {
	HasCaps = func() bool { return true }
	esc := New().(*sudoEscalator)
	if esc.useSudo {
		t.Fatalf("expected capabilities to be used")
	}
	if err := esc.Ensure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureWithSudo(t *testing.T) {
	HasCaps = func() bool { return false }
	lookPath = fakeLookPath(nil)
	esc := New().(*sudoEscalator)
	if !esc.useSudo {
		t.Fatalf("expected sudo usage")
	}
	if err := esc.Ensure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureNoSudo(t *testing.T) {
	HasCaps = func() bool { return false }
	lookPath = fakeLookPath(errors.New("missing"))
	esc := New().(*sudoEscalator)
	if err := esc.Ensure(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCommand(t *testing.T) {
	HasCaps = func() bool { return false }
	esc := New()
	cmd := esc.Command("echo", "hi")
	if cmd.Args[0] != "sudo" {
		t.Fatalf("expected sudo prefix")
	}
	HasCaps = func() bool { return true }
	esc = New()
	cmd = esc.Command("echo", "hi")
	if cmd.Args[0] == "sudo" {
		t.Fatalf("unexpected sudo prefix")
	}
}

// Restore globals after tests.
func TestMain(m *testing.M) {
	code := m.Run()
	HasCaps = RealHasCaps
	lookPath = exec.LookPath
	if code != 0 {
		panic(code)
	}
}
