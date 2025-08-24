//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/sys/unix"

	"lvmsync_go/device"
)

// helper to create executable script with provided body
func helperScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")
	content := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

// create mock block device using mknod with major 1 minor 0
func mockBlockDevice(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "blockdev")
	dev := int(unix.Mkdev(1, 0))
	if err := unix.Mknod(path, unix.S_IFBLK|0o600, dev); err != nil {
		t.Fatalf("mknod: %v", err)
	}
	return path
}

func runOpenRaw(t *testing.T, freeze, thaw string, timeout time.Duration) (*observer.ObservedLogs, error) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	ctx := device.WithForce(context.Background(), true)
	ctx = device.WithAllowOverwrite(ctx, true)
	ctx = device.WithYesIKnow(ctx, true)
	devPath := mockBlockDevice(t)
	_, err := device.OpenRaw(ctx, devPath, false, false, freeze, nil, thaw, nil, timeout, time.Second, nil, logger, device.NewRunner())
	return logs, err
}

func TestFreezeHelpers(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	thaw := helperScript(t, "exit 0")

	t.Run("success", func(t *testing.T) {
		freeze := helperScript(t, "exit 0")
		logs, err := runOpenRaw(t, freeze, thaw, time.Second)
		if err != nil {
			t.Fatalf("open raw: %v", err)
		}
		if logs.FilterMessage("fs_freeze_start").Len() != 1 || logs.FilterMessage("fs_freeze_complete").Len() != 1 {
			t.Fatalf("expected freeze start and complete logs, got %v", logs.All())
		}
		if logs.FilterMessage("fs_freeze_failed").Len() != 0 {
			t.Fatalf("unexpected freeze_failed log")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		freeze := helperScript(t, "sleep 2")
		logs, err := runOpenRaw(t, freeze, thaw, 500*time.Millisecond)
		if err == nil {
			t.Fatalf("expected timeout error")
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected exit error, got %v", err)
		}
		if exitErr.ExitCode() == 0 {
			t.Fatalf("expected non-zero exit code")
		}
		if logs.FilterMessage("fs_freeze_start").Len() != 1 || logs.FilterMessage("fs_freeze_failed").Len() != 1 {
			t.Fatalf("expected freeze start and failed logs, got %v", logs.All())
		}
		if logs.FilterMessage("fs_freeze_complete").Len() != 0 {
			t.Fatalf("unexpected freeze complete log")
		}
	})

	t.Run("failure", func(t *testing.T) {
		freeze := helperScript(t, "echo fail >&2\nexit 3")
		logs, err := runOpenRaw(t, freeze, thaw, time.Second)
		if err == nil {
			t.Fatalf("expected failure")
		}
		if !strings.Contains(err.Error(), "exit status") {
			t.Fatalf("expected exit status in error, got %v", err)
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
			t.Fatalf("expected non-zero exit code, got %v", err)
		}
		if logs.FilterMessage("fs_freeze_start").Len() != 1 || logs.FilterMessage("fs_freeze_failed").Len() != 1 {
			t.Fatalf("expected freeze start and failed logs, got %v", logs.All())
		}
		if logs.FilterMessage("fs_freeze_complete").Len() != 0 {
			t.Fatalf("unexpected freeze complete log")
		}
	})
}
