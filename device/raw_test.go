package device

import (
	"context"
	"fmt"
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
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop(), thawCmdPath: "touch", thawCmdArgs: []string{tmp}}
	if err := d.Cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("thaw command did not run")
	}
}

func TestCleanupThawCommandFailure(t *testing.T) {
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop(), thawCmdPath: "false"}
	if err := d.Cleanup(context.Background()); err == nil {
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
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop(), thawTimeout: 100 * time.Millisecond, thawCmdPath: "sleep", thawCmdArgs: []string{"2"}}
	if err := d.Cleanup(context.Background()); err == nil || !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("expected thaw command to be killed, got %v", err)
	}
}

func TestOpenRawStoresThawConfig(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	thawTmp := filepath.Join(t.TempDir(), "thaw")
	d, err := OpenRaw(context.Background(), loop, false, "true", nil, "touch", []string{thawTmp}, time.Second, time.Second, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if d.thawCmdPath != "touch" || len(d.thawCmdArgs) != 1 || d.thawCmdArgs[0] != thawTmp {
		t.Fatalf("thaw configuration not stored")
	}
	if err := d.Cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(thawTmp); err != nil {
		t.Fatalf("thaw command did not run")
	}
	d.Close()
}

func TestOpenRawFreezeThawLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	oldExec := execCommand
	execCommand = fakeExecCommandContext
	defer func() { execCommand = oldExec }()
	_, err := OpenRaw(context.Background(), "/dev/null", false, os.Args[0], []string{"freeze-success"}, os.Args[0], []string{"thaw-success"}, time.Second, time.Second, logger)
	if err == nil {
		t.Fatalf("expected error for char device")
	}
	if logs.FilterMessage("fs freeze start").Len() != 1 || logs.FilterMessage("fs freeze complete").Len() != 1 {
		t.Fatalf("expected freeze start and complete logs, got %v", logs.All())
	}
	if logs.FilterMessage("fs thaw start").Len() != 1 || logs.FilterMessage("fs thaw complete").Len() != 1 {
		t.Fatalf("expected thaw start and complete logs, got %v", logs.All())
	}
}

func TestOpenRawFreezeTimeoutLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	oldExec := execCommand
	execCommand = fakeExecCommandContext
	defer func() { execCommand = oldExec }()
	_, err := OpenRaw(context.Background(), "/dev/null", false, os.Args[0], []string{"freeze-timeout"}, os.Args[0], []string{"thaw-success"}, 50*time.Millisecond, time.Second, logger)
	if err == nil || !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("expected freeze timeout, got %v", err)
	}
	if logs.FilterMessage("fs freeze start").Len() != 1 {
		t.Fatalf("expected freeze start log")
	}
	if logs.FilterMessage("fs freeze complete").Len() != 0 {
		t.Fatalf("unexpected freeze complete log")
	}
	if logs.FilterMessage("fs thaw start").Len() != 0 || logs.FilterMessage("fs thaw complete").Len() != 0 {
		t.Fatalf("unexpected thaw logs, got %v", logs.All())
	}
}

func TestOpenRawThawFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	oldExec := execCommand
	execCommand = fakeExecCommandContext
	defer func() { execCommand = oldExec }()
	_, err := OpenRaw(context.Background(), "/dev/null", false, os.Args[0], []string{"freeze-success"}, os.Args[0], []string{"thaw-fail"}, time.Second, time.Second, logger)
	if err == nil {
		t.Fatalf("expected error for char device")
	}
	if logs.FilterMessage("fs freeze start").Len() != 1 || logs.FilterMessage("fs freeze complete").Len() != 1 {
		t.Fatalf("expected freeze start and complete logs, got %v", logs.All())
	}
	if logs.FilterMessage("fs thaw start").Len() != 1 {
		t.Fatalf("expected thaw start log")
	}
	if logs.FilterMessage("fs thaw complete").Len() != 0 {
		t.Fatalf("unexpected thaw complete log")
	}
	if logs.FilterMessage("fs thaw failed").Len() != 1 {
		t.Fatalf("expected thaw failed log")
	}
}

func TestOpenRawFreezeCommandFailureIncludesOutput(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	oldExec := execCommand
	execCommand = fakeExecCommandContext
	defer func() { execCommand = oldExec }()
	if _, err := OpenRaw(context.Background(), "/dev/null", false, os.Args[0], []string{"freeze-fail-output"}, "true", nil, time.Second, time.Second, logger); err == nil || !strings.Contains(err.Error(), "freeze output") {
		t.Fatalf("expected freeze output in error, got %v", err)
	}
	entries := logs.FilterMessage("fs freeze failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one freeze failed log, got %v", logs.All())
	}
	if v, ok := entries[0].ContextMap()["output"]; !ok || v != "freeze output" {
		t.Fatalf("expected freeze output log, got %v", entries[0].ContextMap())
	}
}

func TestRawDeviceCleanupFailureIncludesOutput(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	oldExec := execCommand
	execCommand = fakeExecCommandContext
	defer func() { execCommand = oldExec }()
	d := &RawDevice{
		freezeIssued: true,
		thawCmdPath:  os.Args[0],
		thawCmdArgs:  []string{"thaw-fail-output"},
		thawTimeout:  time.Second,
		logger:       logger,
	}
	if err := d.Cleanup(context.Background()); err == nil || !strings.Contains(err.Error(), "thaw output") {
		t.Fatalf("expected thaw output in error, got %v", err)
	}
	entries := logs.FilterMessage("fs thaw failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one thaw failed log, got %v", logs.All())
	}
	if v, ok := entries[0].ContextMap()["output"]; !ok || v != "thaw output" {
		t.Fatalf("expected thaw output log, got %v", entries[0].ContextMap())
	}
}

func TestRawDeviceCleanupThawErrorLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	oldExec := execCommand
	execCommand = fakeExecCommandContext
	defer func() { execCommand = oldExec }()
	d := &RawDevice{
		freezeIssued: true,
		thawCmdPath:  os.Args[0],
		thawCmdArgs:  []string{"thaw-fail"},
		thawTimeout:  time.Second,
		logger:       logger,
	}
	if err := d.Cleanup(context.Background()); err == nil {
		t.Fatalf("expected thaw command failure")
	}
	if logs.FilterMessage("fs thaw failed").Len() != 1 {
		t.Fatalf("expected thaw failed log")
	}
}

func TestRawDeviceCleanupThawTimeoutLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	oldExec := execCommand
	execCommand = fakeExecCommandContext
	defer func() { execCommand = oldExec }()
	d := &RawDevice{
		freezeIssued: true,
		thawCmdPath:  os.Args[0],
		thawCmdArgs:  []string{"thaw-timeout"},
		thawTimeout:  100 * time.Millisecond,
		logger:       logger,
	}
	if err := d.Cleanup(context.Background()); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("expected thaw command to be killed, got %v", err)
	}
	if logs.FilterMessage("fs thaw failed").Len() != 1 {
		t.Fatalf("expected thaw failed log")
	}
}

func fakeExecCommandContext(ctx context.Context, _ string, args ...string) *exec.Cmd {
	cs := append([]string{"-test.run=TestHelperProcess", "--"}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i := range args {
		if args[i] == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	switch args[0] {
	case "freeze-success", "thaw-success":
		os.Exit(0)
	case "freeze-timeout", "thaw-timeout":
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	case "thaw-fail":
		os.Exit(1)
	case "freeze-fail-output":
		fmt.Fprintln(os.Stderr, "freeze output")
		os.Exit(1)
	case "thaw-fail-output":
		fmt.Fprintln(os.Stderr, "thaw output")
		os.Exit(1)
	default:
		os.Exit(1)
	}
}
