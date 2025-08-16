package dump

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	remotetest "lvmsync_go/remote/testutil"
	"lvmsync_go/transfer"
)

// Test that remote post script executes even when dumpChanges fails
func TestRemotePostScriptRunsOnError(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(_ string) int { return 0 }) // cmd is unused
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	cfg, cfgErr := config.DefaultConfig()
	if cfgErr != nil {
		t.Fatalf("DefaultConfig returned error: %v", cfgErr)
	}
	cfg.RemotePreScript = "pre-script"
	cfg.RemotePostScript = "post-script"
	cfg.SSHUser = "test"
	cfg.SSHPort = port
	cfg.SSHKeyPath = remotetest.CreateTempKey(t)
	cfg.KnownHosts = remotetest.CreateKnownHostsFile(t, server)
	cfg.StrictHostKeyCheck = true
	cfg.LVMSyncPath = "lvmsync"

	original := dumpChangesSequential
	dumpChangesSequential = func(_ *transfer.Transfer, c *config.Config, snapshot, source string, out io.Writer) error {
		return io.ErrUnexpectedEOF
	}
	origSum := sumFile
	sumFile = func(string, string) ([32]byte, error) { return [32]byte{}, nil }
	defer func() { dumpChangesSequential = original; sumFile = origSum }()

	dest := host + ":/dev/null"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	origDetect := detectDevice
	detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger) (device.Device, error) {
		return &fakeDevice{path: "/dev/snap"}, nil
	}
	defer func() { detectDevice = origDetect }()
	_, err = Run(ctx, cfg, "/dev/snap", dest, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "dumpChanges") {
		t.Fatalf("expected dumpChanges error, got %v", err)
	}

	cmds := server.Commands()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0] != cfg.RemotePreScript || cmds[3] != cfg.RemotePostScript {
		t.Fatalf("unexpected command order: %v", cmds)
	}
}

// Test that post script is not executed when pre script fails
func TestRemotePostScriptNotRunIfPreScriptFails(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(cmd string) int {
		if cmd == "fail-pre" {
			return 1
		}
		return 0
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	cfg, cfgErr := config.DefaultConfig()
	if cfgErr != nil {
		t.Fatalf("DefaultConfig returned error: %v", cfgErr)
	}
	cfg.RemotePreScript = "fail-pre"
	cfg.RemotePostScript = "post-script"
	cfg.SSHUser = "test"
	cfg.SSHPort = port
	cfg.SSHKeyPath = remotetest.CreateTempKey(t)
	cfg.KnownHosts = remotetest.CreateKnownHostsFile(t, server)
	cfg.StrictHostKeyCheck = true
	cfg.LVMSyncPath = "lvmsync"

	dest := host + ":/dev/null"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	origDetect := detectDevice
	detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger) (device.Device, error) {
		return &fakeDevice{path: "/dev/snap"}, nil
	}
	defer func() { detectDevice = origDetect }()
	_, err = Run(ctx, cfg, "/dev/snap", dest, zap.NewNop())
	if err == nil {
		t.Fatalf("expected error from pre-script")
	}

	cmds := server.Commands()
	if len(cmds) != 1 || cmds[0] != cfg.RemotePreScript {
		t.Fatalf("post script should not run, commands: %v", cmds)
	}
}

// Test that post script context errors are reported distinctly
func TestRemotePostScriptContextError(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(cmd string) int {
		switch {
		case cmd == "lvmsync --version":
			return 0
		case strings.HasPrefix(cmd, "lvmsync --apply - /dev/null"):
			return 0
		case cmd == "slow-post":
			time.Sleep(100 * time.Millisecond)
			return 0
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return 1
		}
	})
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	cfg, cfgErr := config.DefaultConfig()
	if cfgErr != nil {
		t.Fatalf("DefaultConfig returned error: %v", cfgErr)
	}
	cfg.RemotePostScript = "slow-post"
	cfg.SSHUser = "test"
	cfg.SSHPort = port
	cfg.SSHTimeout = 50 * time.Millisecond
	cfg.SSHKeyPath = remotetest.CreateTempKey(t)
	cfg.KnownHosts = remotetest.CreateKnownHostsFile(t, server)
	cfg.StrictHostKeyCheck = true
	cfg.LVMSyncPath = "lvmsync"

	dest := host + ":/dev/null"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	origDetect := detectDevice
	detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger) (device.Device, error) {
		return &fakeDevice{path: "/dev/snap"}, nil
	}
	defer func() { detectDevice = origDetect }()
	_, err = Run(ctx, cfg, "/dev/snap", dest, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "remote post-script context error") {
		t.Fatalf("expected remote post-script context error, got %v", err)
	}
}
