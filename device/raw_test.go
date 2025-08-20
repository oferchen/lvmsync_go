package device

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type ttyReader struct{ io.Reader }

func (t ttyReader) Fd() uintptr { return 0 }

func helperCommand(t *testing.T) string {
	dir := t.TempDir()
	link := filepath.Join(dir, "cmdhelper")
	if err := os.Symlink(os.Args[0], link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return link
}

func TestOpenRawLogsInfoAndClose(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	d, err := OpenRaw(ctx, loop, true, "", nil, "", nil, time.Second, time.Second, fakeEsc{}, logger, NewRunner())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	entries := logs.FilterMessage("raw_device_info").All()
	found := false
	for _, e := range entries {
		if e.ContextMap()["path"] == loop &&
			e.ContextMap()["size_bytes"].(uint64) == d.SizeBytes() &&
			e.ContextMap()["block_size_bytes"].(uint64) == d.BlockSize() {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected raw_device_info log with fields, got %v", logs.All())
	}
	if logs.FilterMessage("raw_device_closed").Len() == 0 {
		t.Fatalf("expected raw_device_closed log")
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
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	d, err := OpenRaw(ctx, loop, true, "", nil, "", nil, time.Second, time.Second, fakeEsc{}, logger, NewRunner())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := d.f.Close(); err != nil {
		t.Fatalf("preclose: %v", err)
	}
	if err := d.Close(); err == nil {
		t.Fatalf("expected close error")
	}
	if logs.FilterMessage("raw_device_close_failed").Len() == 0 {
		t.Fatalf("expected raw_device_close_failed log")
	}
}

func TestOpenRawRejectsRegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	if _, err := OpenRaw(ctx, f.Name(), true, "", nil, "", nil, 0, 0, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil {
		t.Fatalf("expected error for regular file")
	}
}

func TestOpenRawRejectsCharDevice(t *testing.T) {
	if _, err := os.Stat("/dev/null"); err == nil {
		ctx := WithForce(context.Background(), true)
		ctx = WithAllowOverwrite(ctx, true)
		ctx = WithYesIKnow(ctx, true)
		if _, err := OpenRaw(ctx, "/dev/null", true, "", nil, "", nil, 0, 0, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil {
			t.Fatalf("expected error for char device")
		}
	} else if os.IsNotExist(err) {
		t.Skip("/dev/null not found")
	}
}

func TestOpenRawRequiresOfflineOrFreeze(t *testing.T) {
	if _, err := OpenRaw(context.Background(), "/dev/null", false, "", nil, "", nil, time.Second, time.Second, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil {
		t.Fatalf("expected offline or freeze command error")
	}
}

func TestOpenRawEscalatorError(t *testing.T) {
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	if _, err := OpenRaw(ctx, "/dev/null", true, "", nil, "", nil, 0, 0, fakeEsc{err: errors.New("boom")}, zap.NewNop(), NewRunner()); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected escalator error, got %v", err)
	}
}

func TestOpenRawFreezeCommandFailure(t *testing.T) {
	falsePath, err := exec.LookPath("false")
	if err != nil {
		t.Fatalf("missing false binary: %v", err)
	}
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("missing true binary: %v", err)
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	if _, err := OpenRaw(ctx, "/dev/null", false, falsePath, nil, truePath, nil, time.Second, time.Second, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil {
		t.Fatalf("expected freeze command failure")
	}
}

func TestOpenRawThawsOnFailure(t *testing.T) {
	freezeTmp := filepath.Join(t.TempDir(), "freeze")
	thawTmp := filepath.Join(t.TempDir(), "thaw")
	touchPath, err := exec.LookPath("touch")
	if err != nil {
		t.Fatalf("missing touch binary: %v", err)
	}
	freezeArgs := []string{freezeTmp}
	thawArgs := []string{thawTmp}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	if _, err := OpenRaw(ctx, "/dev/null", false, touchPath, freezeArgs, touchPath, thawArgs, time.Second, time.Second, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil {
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
	touchPath, err := exec.LookPath("touch")
	if err != nil {
		t.Fatalf("missing touch binary: %v", err)
	}
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop(), thawCmdPath: touchPath, thawCmdArgs: []string{tmp}, runner: NewRunner()}
	if err := d.Cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("thaw command did not run")
	}
}

func TestCleanupThawCommandFailure(t *testing.T) {
	falsePath, err := exec.LookPath("false")
	if err != nil {
		t.Fatalf("missing false binary: %v", err)
	}
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop(), thawCmdPath: falsePath, runner: NewRunner()}
	if err := d.Cleanup(context.Background()); err == nil {
		t.Fatalf("expected thaw command failure")
	}
}

func TestOpenRawFreezeTimeout(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep command not found")
	}
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("missing true binary: %v", err)
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	_, err = OpenRaw(ctx, "/dev/null", false, sleepPath, []string{"2"}, truePath, nil, 100*time.Millisecond, time.Second, fakeEsc{}, zap.NewNop(), NewRunner())
	if err == nil || !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("expected freeze command to be killed, got %v", err)
	}
}

func TestConfirmOverwriteTTYRaw(t *testing.T) {
	ctx := WithForce(context.Background(), true)
	r := ttyReader{strings.NewReader("yes\n")}
	if err := confirmOverwrite(ctx, r, io.Discard, func(int) bool { return true }); err != nil {
		t.Fatalf("confirmOverwrite: %v", err)
	}
}

func TestConfirmOverwriteNonTTYRaw(t *testing.T) {
	ctx := WithForce(context.Background(), true)
	r := strings.NewReader("yes\n")
	if err := confirmOverwrite(ctx, r, io.Discard, func(int) bool { return true }); err == nil || !strings.Contains(err.Error(), "--allow-overwrite") {
		t.Fatalf("expected allow-overwrite error, got %v", err)
	}
}

func TestConfirmOverwriteRequiresYesIKnowRaw(t *testing.T) {
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	r := strings.NewReader("yes\n")
	if err := confirmOverwrite(ctx, r, io.Discard, func(int) bool { return true }); err == nil || !strings.Contains(err.Error(), "--yes-i-know") {
		t.Fatalf("expected yes-i-know error, got %v", err)
	}
}

func TestCleanupThawTimeout(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep command not found")
	}
	d := &RawDevice{freezeIssued: true, logger: zap.NewNop(), thawTimeout: 100 * time.Millisecond, thawCmdPath: sleepPath, thawCmdArgs: []string{"2"}, runner: NewRunner()}
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
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("missing true binary: %v", err)
	}
	touchPath, err := exec.LookPath("touch")
	if err != nil {
		t.Fatalf("missing touch binary: %v", err)
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	d, err := OpenRaw(ctx, loop, false, truePath, nil, touchPath, []string{thawTmp}, time.Second, time.Second, fakeEsc{}, zap.NewNop(), NewRunner())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if d.thawCmdPath != touchPath || len(d.thawCmdArgs) != 1 || d.thawCmdArgs[0] != thawTmp {
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
	helper := helperCommand(t)
	runner := NewDeviceRunner(cmdFunc(fakeExecCommandContext))
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	_, err := OpenRaw(ctx, "/dev/null", false, helper, []string{"freeze-success"}, helper, []string{"thaw-success"}, time.Second, time.Second, fakeEsc{}, logger, runner)
	if err == nil {
		t.Fatalf("expected error for char device")
	}
	if logs.FilterMessage("fs_freeze_start").Len() != 1 || logs.FilterMessage("fs_freeze_complete").Len() != 1 {
		t.Fatalf("expected fs_freeze_start and fs_freeze_complete logs, got %v", logs.All())
	}
	if logs.FilterMessage("fs_thaw_start").Len() != 1 || logs.FilterMessage("fs_thaw_complete").Len() != 1 {
		t.Fatalf("expected fs_thaw_start and fs_thaw_complete logs, got %v", logs.All())
	}
}

func TestOpenRawFreezeTimeoutLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	helper := helperCommand(t)
	runner := NewDeviceRunner(cmdFunc(fakeExecCommandContext))
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	_, err := OpenRaw(ctx, "/dev/null", false, helper, []string{"freeze-timeout"}, helper, []string{"thaw-success"}, 50*time.Millisecond, time.Second, fakeEsc{}, logger, runner)
	if err == nil || !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("expected freeze timeout, got %v", err)
	}
	if logs.FilterMessage("fs_freeze_start").Len() != 1 {
		t.Fatalf("expected fs_freeze_start log")
	}
	if logs.FilterMessage("fs_freeze_complete").Len() != 0 {
		t.Fatalf("unexpected fs_freeze_complete log")
	}
	if logs.FilterMessage("fs_thaw_start").Len() != 0 || logs.FilterMessage("fs_thaw_complete").Len() != 0 {
		t.Fatalf("unexpected fs_thaw_* logs, got %v", logs.All())
	}
}

func TestOpenRawFreezeTimeoutErrorLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	helper := helperCommand(t)
	runner := NewDeviceRunner(cmdFunc(fakeExecCommandContext))
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	_, err := OpenRaw(ctx, "/dev/null", false, helper, []string{"freeze-timeout"}, helper, []string{"thaw-success"}, 50*time.Millisecond, time.Second, fakeEsc{}, logger, runner)
	if err == nil || !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("expected freeze timeout, got %v", err)
	}
	if logs.FilterMessage("fs_freeze_start").Len() != 1 {
		t.Fatalf("expected fs_freeze_start log")
	}
	if logs.FilterMessage("fs_freeze_failed").Len() != 1 {
		t.Fatalf("expected fs_freeze_failed log")
	}
	if logs.FilterMessage("fs_freeze_complete").Len() != 0 {
		t.Fatalf("unexpected fs_freeze_complete log")
	}
	if logs.FilterMessage("fs_thaw_start").Len() != 0 || logs.FilterMessage("fs_thaw_complete").Len() != 0 {
		t.Fatalf("unexpected fs_thaw_* logs, got %v", logs.All())
	}
}

func TestOpenRawThawFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	helper := helperCommand(t)
	runner := NewDeviceRunner(cmdFunc(fakeExecCommandContext))
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	_, err := OpenRaw(ctx, "/dev/null", false, helper, []string{"freeze-success"}, helper, []string{"thaw-fail"}, time.Second, time.Second, fakeEsc{}, logger, runner)
	if err == nil {
		t.Fatalf("expected error for char device")
	}
	if logs.FilterMessage("fs_freeze_start").Len() != 1 || logs.FilterMessage("fs_freeze_complete").Len() != 1 {
		t.Fatalf("expected fs_freeze_start and fs_freeze_complete logs, got %v", logs.All())
	}
	if logs.FilterMessage("fs_thaw_start").Len() != 1 {
		t.Fatalf("expected fs_thaw_start log")
	}
	if logs.FilterMessage("fs_thaw_complete").Len() != 0 {
		t.Fatalf("unexpected fs_thaw_complete log")
	}
	if logs.FilterMessage("fs_thaw_failed").Len() != 1 {
		t.Fatalf("expected fs_thaw_failed log")
	}
}

func TestOpenRawFreezeCommandFailureIncludesOutput(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	helper := helperCommand(t)
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("missing true binary: %v", err)
	}
	runner := NewDeviceRunner(cmdFunc(fakeExecCommandContext))
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	if _, err := OpenRaw(ctx, "/dev/null", false, helper, []string{"freeze-fail-output"}, truePath, nil, time.Second, time.Second, fakeEsc{}, logger, runner); err == nil || !strings.Contains(err.Error(), "freeze output") {
		t.Fatalf("expected freeze output in error, got %v", err)
	}
	entries := logs.FilterMessage("fs_freeze_failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one fs_freeze_failed log, got %v", logs.All())
	}
	if v, ok := entries[0].ContextMap()["output"]; !ok || v != "freeze output" {
		t.Fatalf("expected freeze output log, got %v", entries[0].ContextMap())
	}
}

func TestRawDeviceCleanupFailureIncludesOutput(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	helper := helperCommand(t)
	d := &RawDevice{
		freezeIssued: true,
		thawCmdPath:  helper,
		thawCmdArgs:  []string{"thaw-fail-output"},
		thawTimeout:  time.Second,
		logger:       logger,
		runner:       NewDeviceRunner(cmdFunc(fakeExecCommandContext)),
	}
	if err := d.Cleanup(context.Background()); err == nil || !strings.Contains(err.Error(), "thaw output") {
		t.Fatalf("expected thaw output in error, got %v", err)
	}
	entries := logs.FilterMessage("fs_thaw_failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one fs_thaw_failed log, got %v", logs.All())
	}
	if v, ok := entries[0].ContextMap()["output"]; !ok || v != "thaw output" {
		t.Fatalf("expected thaw output log, got %v", entries[0].ContextMap())
	}
}

func TestRawDeviceCleanupThawErrorLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	helper := helperCommand(t)
	d := &RawDevice{
		freezeIssued: true,
		thawCmdPath:  helper,
		thawCmdArgs:  []string{"thaw-fail"},
		thawTimeout:  time.Second,
		logger:       logger,
		runner:       NewDeviceRunner(cmdFunc(fakeExecCommandContext)),
	}
	if err := d.Cleanup(context.Background()); err == nil {
		t.Fatalf("expected thaw command failure")
	}
	if logs.FilterMessage("fs_thaw_failed").Len() != 1 {
		t.Fatalf("expected fs_thaw_failed log")
	}
}

func TestRawDeviceCleanupThawTimeoutLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	helper := helperCommand(t)
	d := &RawDevice{
		freezeIssued: true,
		thawCmdPath:  helper,
		thawCmdArgs:  []string{"thaw-timeout"},
		thawTimeout:  100 * time.Millisecond,
		logger:       logger,
		runner:       NewDeviceRunner(cmdFunc(fakeExecCommandContext)),
	}
	if err := d.Cleanup(context.Background()); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("expected thaw command to be killed, got %v", err)
	}
	if logs.FilterMessage("fs_thaw_failed").Len() != 1 {
		t.Fatalf("expected fs_thaw_failed log")
	}
}

func TestRawDeviceCleanupThawTimeoutErrorLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	helper := helperCommand(t)
	d := &RawDevice{
		freezeIssued: true,
		thawCmdPath:  helper,
		thawCmdArgs:  []string{"thaw-timeout"},
		thawTimeout:  100 * time.Millisecond,
		logger:       logger,
		runner:       NewDeviceRunner(cmdFunc(fakeExecCommandContext)),
	}
	if err := d.Cleanup(context.Background()); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("expected thaw command to be killed, got %v", err)
	}
	if logs.FilterMessage("fs_thaw_start").Len() != 1 {
		t.Fatalf("expected fs_thaw_start log")
	}
	if logs.FilterMessage("fs_thaw_failed").Len() != 1 {
		t.Fatalf("expected fs_thaw_failed log")
	}
	if logs.FilterMessage("fs_thaw_complete").Len() != 0 {
		t.Fatalf("unexpected fs_thaw_complete log")
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
