package privesc

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// dropPriv changes the effective UID for the duration of the test. It skips the
// test if privileges cannot be adjusted.
func dropPriv(t *testing.T) func() {
	t.Helper()
	orig := os.Geteuid()
	if orig != 0 {
		t.Skip("test requires root")
	}
	if err := unix.Setresuid(-1, 1000, -1); err != nil {
		t.Skipf("unable to drop privileges: %v", err)
	}
	return func() {
		_ = unix.Setresuid(-1, orig, -1)
	}
}

func TestEnsureRootAlreadyRoot(t *testing.T) {
	called := false
	err := EnsureRoot("sudo -n", func(argv0 string, argv []string, envv []string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if called {
		t.Fatal("exec function should not be called when already root")
	}
}

func TestEnsureRootMissingCommand(t *testing.T) {
	restore := dropPriv(t)
	defer restore()

	called := false
	err := EnsureRoot("", func(argv0 string, argv []string, envv []string) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if called {
		t.Fatal("exec function should not be called when command is missing")
	}
}

func TestEnsureRootExecFailure(t *testing.T) {
	restore := dropPriv(t)
	defer restore()

	boom := errors.New("boom")
	err := EnsureRoot("sudo -n", func(argv0 string, argv []string, envv []string) error {
		return boom
	})
	if err == nil {
		t.Fatal("expected exec failure")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom error, got %v", err)
	}
}
