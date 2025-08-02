// main.go
package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"lvmsync_go/config"
	"lvmsync_go/lvm"
	"lvmsync_go/remote"
	"lvmsync_go/transfer"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// Interfaces and helpers to allow injection of fake SSH clients in tests.
type SSHClient interface {
	NewSession() (SSHSession, error)
	Close() error
}

type SSHSession interface {
	StdoutPipe() (io.Reader, error)
	StderrPipe() (io.Reader, error)
	StdinPipe() (io.WriteCloser, error)
	Start(cmd string) error
	Wait() error
	Close() error
}

type realSSHClient struct{ *ssh.Client }

func (c *realSSHClient) NewSession() (SSHSession, error) {
	s, err := c.Client.NewSession()
	if err != nil {
		return nil, err
	}
	return &realSSHSession{s}, nil
}

func (c *realSSHClient) Close() error { return c.Client.Close() }

type realSSHSession struct{ *ssh.Session }

func (s *realSSHSession) StdoutPipe() (io.Reader, error)     { return s.Session.StdoutPipe() }
func (s *realSSHSession) StderrPipe() (io.Reader, error)     { return s.Session.StderrPipe() }
func (s *realSSHSession) StdinPipe() (io.WriteCloser, error) { return s.Session.StdinPipe() }
func (s *realSSHSession) Start(cmd string) error             { return s.Session.Start(cmd) }
func (s *realSSHSession) Wait() error                        { return s.Session.Wait() }
func (s *realSSHSession) Close() error                       { return s.Session.Close() }

var (
	newSSHClient = func(host, user, keyPath string, port int, knownHostsPath string, verify bool, timeout, keepAliveInterval time.Duration, retries int) (SSHClient, error) {
		c, err := remote.NewSSHClient(host, user, keyPath, port, knownHostsPath, verify, timeout, keepAliveInterval, retries)
		if err != nil {
			return nil, err
		}
		return &realSSHClient{c}, nil
	}

	validateRemoteCommand = func(client SSHClient, cmd string) error {
		if rc, ok := client.(*realSSHClient); ok {
			return remote.ValidateRemoteCommand(rc.Client, cmd)
		}
		return nil
	}

	runRemoteScript = func(client SSHClient, script string) error {
		if rc, ok := client.(*realSSHClient); ok {
			return remote.RunRemoteScript(rc.Client, script)
		}
		return nil
	}

	dumpChangesSequential = transfer.DumpChangesSequential
	dumpChangesParallel   = transfer.DumpChangesParallel
	dumpChangesWithDedup  = transfer.DumpChangesWithDeduplication
	runApply              = transfer.RunApply
)

func runApplyMode(cfg *config.Config, applyFile, destDevice string) error {
	if applyFile == "" {
		applyFile = "-"
	}
	return runApply(cfg, applyFile, destDevice)
}

func runClientMode(cfg *config.Config, snapshotDevice, dest string) error {
	originDevice := snapshotDevice
	if strings.Contains(dest, ":") {
		parts := strings.SplitN(dest, ":", 2)
		destHost := parts[0]
		destDevice := parts[1]
		client, err := newSSHClient(destHost, cfg.SSHUser, cfg.SSHKeyPath, cfg.SSHPort, cfg.KnownHosts, cfg.StrictHostKeyCheck, cfg.SSHTimeout, cfg.SSHKeepAliveInterval, cfg.MaxRetries)
		if err != nil {
			return fmt.Errorf("failed to create SSH client: %w", err)
		}
		defer client.Close()
		remote.SetLogger(zap.L())

		if cfg.RemotePreScript != "" {
			if err := runRemoteScript(client, cfg.RemotePreScript); err != nil {
				return fmt.Errorf("remote pre-script failed: %w", err)
			}
		}
		if err := validateRemoteCommand(client, cfg.LVMSyncPath); err != nil {
			return fmt.Errorf("remote command validation failed: %w", err)
		}

		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create SSH session: %w", err)
		}
		defer session.Close()

		stdoutPipe, err := session.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to get stdout pipe: %w", err)
		}
		stderrPipe, err := session.StderrPipe()
		if err != nil {
			return fmt.Errorf("failed to get stderr pipe: %w", err)
		}
		go io.Copy(os.Stdout, stdoutPipe)
		go io.Copy(os.Stderr, stderrPipe)

		remoteCmd := fmt.Sprintf("%s --apply - %s", cfg.LVMSyncPath, destDevice)
		zap.L().Info("Starting remote apply command", zap.String("command", remoteCmd))

		remoteStdin, err := session.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to get remote stdin: %w", err)
		}
		if err := session.Start(remoteCmd); err != nil {
			return fmt.Errorf("failed to start remote command: %w", err)
		}

		var streamErr error
		if cfg.Deduplication {
			dedup := transfer.NewDeduplicationStrategy(cfg)
			defer dedup.SaveState()
			streamErr = dumpChangesWithDedup(cfg, snapshotDevice, originDevice, remoteStdin, dedup)
		} else {
			if cfg.Parallel <= 1 {
				streamErr = dumpChangesSequential(cfg, snapshotDevice, originDevice, remoteStdin)
			} else {
				streamErr = dumpChangesParallel(cfg, snapshotDevice, originDevice, remoteStdin)
			}
		}

		remoteStdin.Close()
		if streamErr != nil {
			return fmt.Errorf("error during dumpChanges: %w", streamErr)
		}

		if err := session.Wait(); err != nil {
			return fmt.Errorf("remote command error: %w", err)
		}

		if cfg.RemotePostScript != "" {
			if err := runRemoteScript(client, cfg.RemotePostScript); err != nil {
				return fmt.Errorf("remote post-script failed: %w", err)
			}
		}
	} else {
		destFile, err := os.OpenFile(dest, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("failed to open destination device %s: %w", dest, err)
		}
		defer destFile.Close()

		limitedOut := transfer.WrapRateLimitedWriter(destFile, cfg.SpeedLimit)

		if cfg.Deduplication {
			dedup := transfer.NewDeduplicationStrategy(cfg)
			defer dedup.SaveState()
			return dumpChangesWithDedup(cfg, snapshotDevice, originDevice, limitedOut, dedup)
		}
		if cfg.Parallel <= 1 {
			return dumpChangesSequential(cfg, snapshotDevice, originDevice, limitedOut)
		}
		return dumpChangesParallel(cfg, snapshotDevice, originDevice, limitedOut)
	}
	return nil
}

func handleSignals(cfg *config.Config, signals <-chan os.Signal, snapshotPath *string) {
	sig := <-signals
	zap.L().Info("Received signal, aborting", zap.String("signal", sig.String()))
	if !cfg.SkipSnapshotCreation && *snapshotPath != "" && len(pflag.Args()) > 0 && *snapshotPath != pflag.Arg(0) {
		if err := lvm.RemoveSnapshot(*snapshotPath); err != nil {
			zap.L().Warn("Failed to remove snapshot on shutdown", zap.Error(err))
		} else {
			zap.L().Info("Snapshot removed on shutdown", zap.String("snapshot", *snapshotPath))
		}
	}
	os.Exit(1)
}

// Run executes the core logic given a configuration and arguments.
func Run(cfg *config.Config, args []string) error {
	lvm.SetEscalationCommand(cfg.LVMEscalation)

	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("logger initialization error: %w", err)
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	var snapshotPath string
	go handleSignals(cfg, signals, &snapshotPath)

	if cfg.ApplyMode != "" {
		if len(args) < 1 {
			return fmt.Errorf("no destination device specified for apply mode")
		}
		return runApplyMode(cfg, cfg.ApplyMode, args[0])
	}

	if len(args) < 2 {
		return fmt.Errorf("expected source and destination arguments")
	}
	originalVolume := args[0]
	destPath := args[1]

	if !cfg.SkipDiskCheck {
		freeSpace, err := lvm.CheckDiskSpace("/")
		if err != nil {
			return fmt.Errorf("disk space check failed: %w", err)
		}
		requiredBytes, err := lvm.ParseSnapshotSize(cfg.SnapshotSize, originalVolume)
		if err != nil {
			return fmt.Errorf("failed to parse snapshot size: %w", err)
		}
		if freeSpace < requiredBytes {
			return fmt.Errorf("insufficient disk space for snapshot")
		}
		zap.L().Info("Disk space check passed", zap.Uint64("free", freeSpace))
	}

	snapshotPath = originalVolume
	if !cfg.SkipSnapshotCreation {
		snapshotName := fmt.Sprintf("snap-%d", time.Now().Unix())
		if err := lvm.CreateSnapshot(originalVolume, snapshotName, cfg.SnapshotSize); err != nil {
			return fmt.Errorf("snapshot creation failed: %w", err)
		}
		snapshotPath = lvm.GetSnapshotDevicePath(snapshotName, cfg.VolumeGroup)
		zap.L().Info("Snapshot created", zap.String("snapshot", snapshotPath))

		stopMonitor := make(chan struct{})
		go func() {
			if err := lvm.MonitorSnapshot(snapshotPath, 80.0, 10*time.Second, stopMonitor); err != nil {
				zap.L().Error("Snapshot monitor error", zap.Error(err))
				os.Exit(1)
			}
		}()
		defer close(stopMonitor)
	}

	if err := runClientMode(cfg, snapshotPath, destPath); err != nil {
		return fmt.Errorf("copy operation failed: %w", err)
	}

	if !cfg.SkipSnapshotCreation {
		if err := lvm.RemoveSnapshot(snapshotPath); err != nil {
			zap.L().Warn("Failed to remove snapshot", zap.Error(err))
		} else {
			zap.L().Info("Snapshot removed", zap.String("snapshot", snapshotPath))
		}
	}
	return nil
}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation error: %v\n", err)
		os.Exit(1)
	}

	if err := Run(cfg, pflag.Args()); err != nil {
		zap.L().Fatal("Execution failed", zap.Error(err))
	}
}
