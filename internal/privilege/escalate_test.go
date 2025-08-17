package privilege

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

type cmdFunc func(context.Context, string, ...string) *exec.Cmd

func (f cmdFunc) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return f(ctx, name, args...)
}

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
	r := &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(0)(name, args...)
	}), LookPath: fakeLookPath(nil)}
	esc := NewWithRunner(r).(*sudoEscalator)
	if !esc.useSudo {
		t.Fatalf("expected sudo usage")
	}
	if err := esc.Ensure(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureNoSudo(t *testing.T) {
	HasCaps = func() bool { return false }
	esc := NewWithRunner(&Runner{LookPath: fakeLookPath(errors.New("missing"))}).(*sudoEscalator)
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

func TestEnsureSudoFailure(t *testing.T) {
	HasCaps = func() bool { return false }
	r := &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(1)(name, args...)
	}), LookPath: fakeLookPath(nil)}
	esc := NewWithRunner(r).(*sudoEscalator)
	if err := esc.Ensure(); err == nil {
		t.Fatalf("expected error")
	}
}

// fakeSudo stubs exec.Command to emulate sudo exit codes.
func fakeSudo(code int) func(string, ...string) *exec.Cmd {
	return func(string, ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", strconv.Itoa(code)}
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
}

func TestSudoSuccess(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	HasCaps = func() bool { return false }
	esc := NewWithRunner(&Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(0)(name, args...)
	})})
	cmd := esc.Command("echo")
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSudoPermissionDenied(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	HasCaps = func() bool { return false }
	esc := NewWithRunner(&Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(1)(name, args...)
	})})
	cmd := esc.Command("echo")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected error")
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSudoCommandNotFound(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	HasCaps = func() bool { return false }
	esc := NewWithRunner(&Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(127)(name, args...)
	})})
	cmd := esc.Command("echo")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected error")
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 127 {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHelperProcess allows exec.Command stubbing.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code, _ := strconv.Atoi(os.Args[len(os.Args)-1])
	os.Exit(code)
}

// Restore globals after tests.
func TestMain(m *testing.M) {
	code := m.Run()
	HasCaps = RealHasCaps
	if code != 0 {
		panic(code)
	}
}
