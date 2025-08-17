package privilege

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
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
	esc := New(context.Background()).(*sudoEscalator)
	if esc.useSudo {
		t.Fatalf("expected capabilities to be used")
	}
	if err := esc.Ensure(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureWithSudo(t *testing.T) {
	HasCaps = func() bool { return false }
	r := &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(0)(name, args...)
	}), LookPath: fakeLookPath(nil)}
	esc := NewWithRunner(context.Background(), r).(*sudoEscalator)
	if !esc.useSudo {
		t.Fatalf("expected sudo usage")
	}
	if err := esc.Ensure(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureNoSudo(t *testing.T) {
	HasCaps = func() bool { return false }
	esc := NewWithRunner(context.Background(), &Runner{LookPath: fakeLookPath(errors.New("missing"))}).(*sudoEscalator)
	if err := esc.Ensure(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCommand(t *testing.T) {
	HasCaps = func() bool { return false }
	esc := New(context.Background())
	cmd := esc.Command(context.Background(), "echo", "hi")
	if cmd.Args[0] != "sudo" {
		t.Fatalf("expected sudo prefix")
	}
	HasCaps = func() bool { return true }
	esc = New(context.Background())
	cmd = esc.Command(context.Background(), "echo", "hi")
	if cmd.Args[0] == "sudo" {
		t.Fatalf("unexpected sudo prefix")
	}
}

func TestEnsureSudoFailure(t *testing.T) {
	HasCaps = func() bool { return false }
	r := &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(1)(name, args...)
	}), LookPath: fakeLookPath(nil)}
	esc := NewWithRunner(context.Background(), r).(*sudoEscalator)
	if err := esc.Ensure(context.Background()); err == nil {
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
	if _, err := exec.LookPath("sudo"); err != nil {
		t.Skip("sudo not available")
	}
	HasCaps = func() bool { return false }
	esc := NewWithRunner(context.Background(), &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(0)(name, args...)
	})})
	cmd := esc.Command(context.Background(), "echo")
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSudoPermissionDenied(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		t.Skip("sudo not available")
	}
	HasCaps = func() bool { return false }
	esc := NewWithRunner(context.Background(), &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(1)(name, args...)
	})})
	cmd := esc.Command(context.Background(), "echo")
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
	if _, err := exec.LookPath("sudo"); err != nil {
		t.Skip("sudo not available")
	}
	HasCaps = func() bool { return false }
	esc := NewWithRunner(context.Background(), &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeSudo(127)(name, args...)
	})})
	cmd := esc.Command(context.Background(), "echo")
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
	os.Exit(code)
}

func TestEnsureContextCanceled(t *testing.T) {
	HasCaps = func() bool { return false }
	r := &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "10")
	}), LookPath: fakeLookPath(nil)}
	esc := NewWithRunner(context.Background(), r)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := esc.Ensure(ctx)
	if err == nil || ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("expected context deadline exceeded, got err=%v ctxErr=%v", err, ctx.Err())
	}
}

func TestCommandContextCanceled(t *testing.T) {
	esc := &sudoEscalator{useSudo: false, runner: &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "10")
	})}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	cmd := esc.Command(ctx, "sleep", "10")
	err := cmd.Run()
	if err == nil || ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("expected context deadline exceeded, got err=%v ctxErr=%v", err, ctx.Err())
	}
}

func TestEnsureDefaultTimeout(t *testing.T) {
	HasCaps = func() bool { return false }
	orig := escalationTimeout
	escalationTimeout = 10 * time.Millisecond
	defer func() { escalationTimeout = orig }()
	r := &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "10")
	}), LookPath: fakeLookPath(nil)}
	esc := NewWithRunner(context.Background(), r)
	err := esc.Ensure(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestCommandDefaultTimeout(t *testing.T) {
	orig := escalationTimeout
	escalationTimeout = 10 * time.Millisecond
	defer func() { escalationTimeout = orig }()
	esc := &sudoEscalator{useSudo: false, runner: &Runner{Cmd: cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "10")
	})}}
	cmd := esc.Command(context.Background(), "sleep", "10")
	err := cmd.Run()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
