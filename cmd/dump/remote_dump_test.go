package dump

import (
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	remotetest "lvmsync_go/remote/testutil"
	"lvmsync_go/transfer"
)

// TestRunRemoteDump executes a remote apply command through SSH.
func TestRunRemoteDump(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(cmd string) int {
		switch {
		case cmd == "lvmsync --version":
			return 0
		case strings.HasPrefix(cmd, "lvmsync --apply - /dev/null"):
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

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.SSHUser = "test"
	cfg.SSHPort = port
	cfg.SSHKeyPath = remotetest.CreateTempKey(t)
	cfg.KnownHosts = remotetest.CreateKnownHostsFile(t, server)
	cfg.StrictHostKeyCheck = true
	cfg.LVMSyncPath = "lvmsync"
	cfg.Parallel = 1

	origDump := dumpChangesSequential
	dumpChangesSequential = func(_ *transfer.Transfer, c *config.Config, snap, origin string, out io.Writer) error {
		if snap != "snap" || origin != "origin" {
			t.Fatalf("unexpected devices: %s %s", snap, origin)
		}
		return nil
	}
	defer func() { dumpChangesSequential = origDump }()

	dest := host + ":/dev/null"
	if err := RunRemoteDump(cfg, "snap", "origin", dest, zap.NewNop()); err != nil {
		t.Fatalf("runRemoteDump returned error: %v", err)
	}

	cmds := server.Commands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0] != "lvmsync --version" || cmds[1] != "lvmsync --apply - /dev/null" {
		t.Fatalf("unexpected commands: %v", cmds)
	}
}

// TestRunRemoteDumpError verifies that remote command errors are propagated.
func TestRunRemoteDumpError(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(cmd string) int {
		if cmd == "lvmsync --version" {
			return 0
		}
		if strings.HasPrefix(cmd, "lvmsync --apply - /dev/null") {
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

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.SSHUser = "test"
	cfg.SSHPort = port
	cfg.SSHKeyPath = remotetest.CreateTempKey(t)
	cfg.KnownHosts = remotetest.CreateKnownHostsFile(t, server)
	cfg.StrictHostKeyCheck = true
	cfg.LVMSyncPath = "lvmsync"
	cfg.Parallel = 1

	origDump := dumpChangesSequential
	dumpChangesSequential = func(_ *transfer.Transfer, c *config.Config, snap, origin string, out io.Writer) error {
		return nil
	}
	defer func() { dumpChangesSequential = origDump }()

	dest := host + ":/dev/null"
	err = RunRemoteDump(cfg, "snap", "origin", dest, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "remote command error") {
		t.Fatalf("expected remote command error, got %v", err)
	}
}
