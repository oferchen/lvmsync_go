package device

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestOpenRawLogsInfoAndClose(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	d, err := OpenRaw(context.Background(), loop, true, "", nil, "", nil, time.Second, time.Second, logger)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	entries := logs.FilterMessage("raw device info").All()
	found := false
	for _, e := range entries {
		if e.ContextMap()["path"] == loop &&
			e.ContextMap()["size_bytes"].(uint64) == d.SizeBytes() &&
			e.ContextMap()["block_size_bytes"].(uint64) == d.BlockSize() {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected raw device info log with fields, got %v", logs.All())
	}
	if logs.FilterMessage("raw device closed").Len() == 0 {
		t.Fatalf("expected raw device closed log")
	}
}

func TestRawDeviceCloseErrorLogging(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	d, err := OpenRaw(context.Background(), loop, true, "", nil, "", nil, time.Second, time.Second, logger)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := d.f.Close(); err != nil {
		t.Fatalf("preclose: %v", err)
	}
	if err := d.Close(); err == nil {
		t.Fatalf("expected close error")
	}
	if logs.FilterMessage("raw device close failed").Len() == 0 {
		t.Fatalf("expected raw device close failed log")
	}
}

func TestOpenRawRejectsRegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	if _, err := OpenRaw(context.Background(), f.Name(), true, "", nil, "", nil, 0, 0, nil); err == nil {
		t.Fatalf("expected error for regular file")
	}
}

func TestOpenRawRejectsCharDevice(t *testing.T) {
	if _, err := os.Stat("/dev/null"); err == nil {
		if _, err := OpenRaw(context.Background(), "/dev/null", true, "", nil, "", nil, 0, 0, nil); err == nil {
			t.Fatalf("expected error for char device")
		}
	} else if os.IsNotExist(err) {
		t.Skip("/dev/null not found")
	}
}

func TestOpenRawRequiresOfflineOrFreeze(t *testing.T) {
	if _, err := OpenRaw(context.Background(), "/dev/null", false, "", nil, "", nil, time.Second, time.Second, nil); err == nil {
		t.Fatalf("expected offline or freeze command error")
	}
}

func TestOpenRawFreezeCommandFailure(t *testing.T) {
	if _, err := OpenRaw(context.Background(), "/dev/null", false, "false", nil, "true", nil, time.Second, time.Second, nil); err == nil {
		t.Fatalf("expected freeze command failure")
	}
}

func TestOpenRawThawsOnFailure(t *testing.T) {
	freezeTmp := filepath.Join(t.TempDir(), "freeze")
	thawTmp := filepath.Join(t.TempDir(), "thaw")
	freezeCmdPath := "touch"
	freezeArgs := []string{freezeTmp}
	thawCmdPath := "touch"
	thawArgs := []string{thawTmp}
	if _, err := OpenRaw(context.Background(), "/dev/null", false, freezeCmdPath, freezeArgs, thawCmdPath, thawArgs, time.Second, time.Second, nil); err == nil {
		t.Fatalf("expected error for char device")
	}
	if _, err := os.Stat(freezeTmp); err != nil {
		t.Fatalf("freeze command did not run")
	}
	if _, err := os.Stat(thawTmp); err != nil {
		t.Fatalf("thaw command did not run")
	}
}

func TestCleanupRunsThawCommand(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "thaw")
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop()}
	if err := d.Cleanup(context.Background(), "touch", []string{tmp}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("thaw command did not run")
	}
}

func TestCleanupThawCommandFailure(t *testing.T) {
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop()}
	if err := d.Cleanup(context.Background(), "false", nil); err == nil {
		t.Fatalf("expected thaw command failure")
	}
}

func TestOpenRawFreezeTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep command not found")
	}
	_, err := OpenRaw(context.Background(), "/dev/null", false, "sleep", []string{"2"}, "true", nil, 100*time.Millisecond, time.Second, nil)
	if err == nil || !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("expected freeze command to be killed, got %v", err)
	}
}

func TestCleanupThawTimeout(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep command not found")
	}
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop(), thawTimeout: 100 * time.Millisecond}
	if err := d.Cleanup(context.Background(), "sleep", []string{"2"}); err == nil || !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("expected thaw command to be killed, got %v", err)
	}
}
