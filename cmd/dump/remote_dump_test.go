package dump

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/config"
	digestpkg "lvmsync_go/internal/digest"
	remotetest "lvmsync_go/remote/testutil"
	"lvmsync_go/transfer"
)

// TestRunRemoteDump executes a remote command through SSH.
func TestRunRemoteDump(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(cmd string) int {
		switch {
		case cmd == "lvmsync --version":
			return 0
		case strings.HasPrefix(cmd, "lvmsync --digest sha256 --verify none /dev/null"):
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
	origSum := sumFile
	origSelect := digestpkg.Select
	origStream := streamToRemote
	digestpkg.Select = func() string { return digestpkg.SHA256 }
	sumFile = func(string, string) ([32]byte, error) { return [32]byte{}, nil }
	streamToRemote = func(_ context.Context, _ *config.Config, _ io.WriteCloser, snap, origin, alg string, _ *zap.Logger) error {
		if snap != "snap" || origin != "origin" || alg != digestpkg.SHA256 {
			t.Fatalf("unexpected params: %s %s %s", snap, origin, alg)
		}
		return nil
	}
	dumpChangesSequential = func(_ context.Context, _ *transfer.Transfer, c *config.Config, snap, origin string, out io.Writer) error {
		if snap != "snap" || origin != "origin" {
			t.Fatalf("unexpected devices: %s %s", snap, origin)
		}
		return nil
	}
	defer func() {
		dumpChangesSequential = origDump
		sumFile = origSum
		digestpkg.Select = origSelect
		streamToRemote = origStream
	}()

	dest := host + ":/dev/null"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := RunRemoteDump(ctx, cfg, "snap", "origin", dest, zap.NewNop()); err != nil {
		t.Fatalf("runRemoteDump returned error: %v", err)
	}

	cmds := server.Commands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0] != "lvmsync --version" || cmds[1] != "lvmsync --digest sha256 --verify none /dev/null" {
		t.Fatalf("unexpected commands: %v", cmds)
	}
}

// TestRunRemoteDumpError verifies that remote command errors are propagated.
func TestRunRemoteDumpError(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(cmd string) int {
		if cmd == "lvmsync --version" {
			return 0
		}
		if strings.HasPrefix(cmd, "lvmsync --digest sha256 --verify none /dev/null") {
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
	origSum := sumFile
	origSelect := digestpkg.Select
	origStream := streamToRemote
	digestpkg.Select = func() string { return digestpkg.SHA256 }
	sumFile = func(string, string) ([32]byte, error) { return [32]byte{}, nil }
	streamToRemote = func(_ context.Context, _ *config.Config, _ io.WriteCloser, snap, origin, alg string, _ *zap.Logger) error {
		if snap != "snap" || origin != "origin" || alg != digestpkg.SHA256 {
			t.Fatalf("unexpected params: %s %s %s", snap, origin, alg)
		}
		return nil
	}
	dumpChangesSequential = func(_ context.Context, _ *transfer.Transfer, c *config.Config, snap, origin string, out io.Writer) error {
		return nil
	}
	defer func() {
		dumpChangesSequential = origDump
		sumFile = origSum
		digestpkg.Select = origSelect
		streamToRemote = origStream
	}()

	dest := host + ":/dev/null"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = RunRemoteDump(ctx, cfg, "snap", "origin", dest, zap.NewNop())
	if err == nil || !errors.Is(err, ErrRemoteCommand) {
		t.Fatalf("expected remote command error, got %v", err)
	}
}

// TestRunRemoteDumpTimeout verifies that command validation respects SSHTimeout.
func TestRunRemoteDumpTimeout(t *testing.T) {
	var cmdCount int
	server := remotetest.NewMockSSHServer(t, func(cmd string) int {
		cmdCount++
		switch {
		case cmd == "lvmsync --version":
			time.Sleep(100 * time.Millisecond)
			return 0
		case strings.HasPrefix(cmd, "lvmsync --digest sha256 --verify none /dev/null"):
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
	cfg.SSHTimeout = 20 * time.Millisecond

	origDump := dumpChangesSequential
	origSum := sumFile
	origStream := streamToRemote
	sumFile = func(string, string) ([32]byte, error) { return [32]byte{}, nil }
	streamToRemote = func(_ context.Context, _ *config.Config, _ io.WriteCloser, snap, origin, alg string, _ *zap.Logger) error {
		return nil
	}
	dumpChangesSequential = func(_ context.Context, _ *transfer.Transfer, c *config.Config, snap, origin string, out io.Writer) error {
		return nil
	}
	defer func() {
		dumpChangesSequential = origDump
		sumFile = origSum
		streamToRemote = origStream
	}()

	dest := host + ":/dev/null"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = RunRemoteDump(ctx, cfg, "snap", "origin", dest, zap.NewNop())
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}

	cmds := server.Commands()
	if len(cmds) != 1 || cmds[0] != "lvmsync --version" {
		t.Fatalf("unexpected commands: %v", cmds)
	}
	if cmdCount != 1 {
		t.Fatalf("expected only validation command, got %d", cmdCount)
	}
}

// TestRunRemoteDumpInvalidDest verifies that an invalid destination format returns an error.
func TestRunRemoteDumpInvalidDest(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	dest := "invalid"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = RunRemoteDump(ctx, cfg, "snap", "origin", dest, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "host:device") {
		t.Fatalf("expected host:device format error, got %v", err)
	}
}

// TestRunRemoteDumpLogsDigestSelection verifies that digest algorithm and CPU
// feature flags are logged when selecting the digest.
func TestRunRemoteDumpLogsDigestSelection(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(cmd string) int {
		switch {
		case cmd == "lvmsync --version":
			return 0
		case strings.HasPrefix(cmd, "lvmsync --digest sha256 --verify none /dev/null"):
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
	cfg.ChecksumAlgorithm = digestpkg.SHA256

	origDump := dumpChangesSequential
	origSum := sumFile
	origStream := streamToRemote
	sumFile = func(string, string) ([32]byte, error) { return [32]byte{}, nil }
	streamToRemote = func(_ context.Context, _ *config.Config, _ io.WriteCloser, snap, origin, alg string, _ *zap.Logger) error {
		return nil
	}
	dumpChangesSequential = func(_ context.Context, _ *transfer.Transfer, c *config.Config, snap, origin string, out io.Writer) error {
		return nil
	}
	defer func() {
		dumpChangesSequential = origDump
		sumFile = origSum
		streamToRemote = origStream
	}()

	dest := host + ":/dev/null"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	if err := RunRemoteDump(ctx, cfg, "snap", "origin", dest, logger); err != nil {
		t.Fatalf("RunRemoteDump returned error: %v", err)
	}

	logs := observed.FilterMessage("digest_selected").All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 digest_selected log, got %d", len(logs))
	}
	fields := logs[0].ContextMap()
	if fields["digest_alg"] != digestpkg.SHA256 {
		t.Fatalf("expected digest_alg %s, got %v", digestpkg.SHA256, fields["digest_alg"])
	}
	for _, key := range []string{"avx2", "avx512", "neon"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("missing %s field", key)
		}
	}
}
