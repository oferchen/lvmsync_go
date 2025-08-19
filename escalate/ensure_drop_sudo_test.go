package escalate

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestEnsureAndDrop_WithSudo runs EnsureRootOrReexec and DropToInvokerIfSudo using a
// stub sudo binary. It verifies that the helper exits successfully and emits
// the expected log fields.
func TestEnsureAndDrop_WithSudo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root to drop privileges")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("/bin/sh not available")
	}

	dir := t.TempDir()
	sudoPath := filepath.Join(dir, "sudo")
	script := "#!/bin/sh\nshift\nif [ \"$1\" = -- ]; then shift; fi\nunset FIRST_RUN\nSUDO_UID=1 SUDO_GID=1 exec \"$@\"\n"
	if err := os.WriteFile(sudoPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write sudo stub: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run", "TestHelperEnsureAndDropWithSudo")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_WITH_SUDO=1",
		"FIRST_RUN=1",
		"PATH="+dir,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper: %v\n%s", err, out.String())
	}
	logs := out.String()
	if !strings.Contains(logs, "\"msg\":\"ensure_root_or_reexec\"") || !strings.Contains(logs, "\"result\":\"reexeced\"") {
		t.Fatalf("missing reexeced log: %s", logs)
	}
	if !strings.Contains(logs, "\"msg\":\"drop_to_invoker_if_sudo\"") || !strings.Contains(logs, "\"result\":\"dropped\"") {
		t.Fatalf("missing dropped log: %s", logs)
	}
}

// TestEnsureAndDrop_NoSudo verifies behaviour when sudo is absent.
func TestEnsureAndDrop_NoSudo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	cmd := exec.Command(os.Args[0], "-test.run", "TestHelperEnsureAndDropNoSudo")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_NO_SUDO=1",
		"PATH=/nonexistent",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected failure when sudo missing\n%s", out.String())
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("unexpected exit: %v (logs: %s)", err, out.String())
	}
	logs := out.String()
	if !strings.Contains(logs, "\"msg\":\"ensure_failed\"") {
		t.Fatalf("missing ensure_failed log: %s", logs)
	}
	if !strings.Contains(logs, "\"msg\":\"drop_to_invoker_if_sudo\"") || !strings.Contains(logs, "\"result\":\"no_sudo\"") {
		t.Fatalf("missing drop_to_invoker_if_sudo no_sudo log: %s", logs)
	}
}

func TestHelperEnsureAndDropWithSudo(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_WITH_SUDO") != "1" {
		return
	}
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		os.Stdout,
		zapcore.InfoLevel,
	))
	defer logger.Sync()
	opts := Options{}
	if os.Getenv("FIRST_RUN") == "1" {
		opts.Geteuid = func() int { return 1 }
	}
	reexeced, err := EnsureRootOrReexec(opts, logger)
	if err != nil {
		logger.Error("ensure_failed", zap.Error(err))
		os.Exit(2)
	}
	if reexeced {
		os.Exit(0)
	}
	if err := DropToInvokerIfSudo(Options{}, logger); err != nil {
		logger.Error("drop_failed", zap.Error(err))
		os.Exit(3)
	}
	os.Exit(0)
}

func TestHelperEnsureAndDropNoSudo(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_NO_SUDO") != "1" {
		return
	}
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		os.Stdout,
		zapcore.InfoLevel,
	))
	defer logger.Sync()
	reexeced, err := EnsureRootOrReexec(Options{Geteuid: func() int { return 1 }}, logger)
	if err != nil {
		logger.Error("ensure_failed", zap.Error(err))
		_ = DropToInvokerIfSudo(Options{}, logger)
		os.Exit(1)
	}
	if reexeced {
		os.Exit(1)
	}
	if err := DropToInvokerIfSudo(Options{}, logger); err != nil {
		logger.Error("drop_failed", zap.Error(err))
		os.Exit(2)
	}
	os.Exit(0)
}
