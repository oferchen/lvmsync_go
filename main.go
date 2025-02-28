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
)

var cfg *config.Config

func runApplyMode() error {
	args := pflag.Args()
	if len(args) < 1 {
		return fmt.Errorf("no destination device specified for apply mode")
	}
	destDevice := args[0]
	return transfer.RunApply(cfg.ApplyMode, destDevice, true, cfg.VerifyChecksum, cfg.Compress)
}

func runClientMode(snapshotDevice, dest string) error {
	originDevice := snapshotDevice
	if strings.Contains(dest, ":") {
		parts := strings.SplitN(dest, ":", 2)
		destHost := parts[0]
		destDevice := parts[1]
		client, err := remote.NewSSHClient(destHost, cfg.SSHUser, cfg.SSHKeyPath, cfg.SSHPort, cfg.KnownHosts, cfg.StrictHostKeyCheck)
		if err != nil {
			return fmt.Errorf("failed to create SSH client: %v", err)
		}
		defer client.Close()
		remote.SetLogger(zap.L())
		if cfg.RemotePreScript != "" {
			if err := remote.RunRemoteScript(client, cfg.RemotePreScript); err != nil {
				return fmt.Errorf("remote pre-script failed: %v", err)
			}
		}
		if err := remote.ValidateRemoteCommand(client, cfg.LVMSyncPath); err != nil {
			return fmt.Errorf("remote command validation failed: %v", err)
		}
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create SSH session: %v", err)
		}
		defer session.Close()
		stdoutPipe, err := session.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to get stdout pipe: %v", err)
		}
		stderrPipe, err := session.StderrPipe()
		if err != nil {
			return fmt.Errorf("failed to get stderr pipe: %v", err)
		}
		go io.Copy(os.Stdout, stdoutPipe)
		go io.Copy(os.Stderr, stderrPipe)
		remoteCmd := fmt.Sprintf("%s --apply - %s", cfg.LVMSyncPath, destDevice)
		zap.L().Info("Starting remote apply command", zap.String("command", remoteCmd))
		remoteStdin, err := session.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to get remote stdin: %v", err)
		}
		if err := session.Start(remoteCmd); err != nil {
			return fmt.Errorf("failed to start remote command: %v", err)
		}
		var streamErr error
		if cfg.Parallel <= 1 {
			streamErr = transfer.DumpChangesSequential(snapshotDevice, originDevice, remoteStdin, cfg.Verbose > 0, cfg.ZeroCopy, cfg.VerifyChecksum, cfg.Compress, cfg.CompressLevel, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
		} else {
			streamErr = transfer.DumpChangesParallel(snapshotDevice, originDevice, remoteStdin, cfg.Verbose > 0, cfg.VerifyChecksum, cfg.Compress, cfg.CompressLevel, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
		}
		remoteStdin.Close()
		if streamErr != nil {
			return fmt.Errorf("error during dumpChanges: %v", streamErr)
		}
		if err := session.Wait(); err != nil {
			return fmt.Errorf("remote command error: %v", err)
		}
		if cfg.RemotePostScript != "" {
			if err := remote.RunRemoteScript(client, cfg.RemotePostScript); err != nil {
				return fmt.Errorf("remote post-script failed: %v", err)
			}
		}
	} else {
		destFile, err := os.OpenFile(dest, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("failed to open destination device %s: %v", dest, err)
		}
		defer destFile.Close()
		limitedOut := transfer.WrapRateLimitedWriter(destFile, cfg.SpeedLimit)
		if cfg.Parallel <= 1 {
			return transfer.DumpChangesSequential(snapshotDevice, originDevice, limitedOut, cfg.Verbose > 0, cfg.ZeroCopy, cfg.VerifyChecksum, cfg.Compress, cfg.CompressLevel, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
		}
		return transfer.DumpChangesParallel(snapshotDevice, originDevice, limitedOut, cfg.Verbose > 0, cfg.VerifyChecksum, cfg.Compress, cfg.CompressLevel, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
	}
	return nil
}

func main() {
	var err error
	cfg, err = config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation error: %v\n", err)
		os.Exit(1)
	}

	lvm.SetEscalationCommand(cfg.LVMEscalation)

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	var snapshotPath string

	go func() {
		sig := <-signals
		zap.L().Info("Received signal, aborting", zap.String("signal", sig.String()))
		if !cfg.SkipSnapshotCreation && snapshotPath != "" && snapshotPath != pflag.Arg(0) {
			if err := lvm.RemoveSnapshot(snapshotPath); err != nil {
				zap.L().Warn("Failed to remove snapshot on shutdown", zap.Error(err))
			} else {
				zap.L().Info("Snapshot removed on shutdown", zap.String("snapshot", snapshotPath))
			}
		}
		os.Exit(1)
	}()

	args := pflag.Args()
	if len(args) < 2 {
		pflag.Usage()
		os.Exit(1)
	}
	originalVolume := args[0]

	if !cfg.SkipDiskCheck {
		freeSpace, err := lvm.CheckDiskSpace("/")
		if err != nil {
			logger.Fatal("Disk space check failed", zap.Error(err))
		}
		requiredBytes, err := lvm.ParseSnapshotSize(cfg.SnapshotSize, originalVolume)
		if err != nil {
			logger.Fatal("Failed to parse snapshot size", zap.Error(err))
		}
		if freeSpace < requiredBytes {
			logger.Fatal("Insufficient disk space for snapshot",
				zap.Uint64("free", freeSpace),
				zap.Uint64("required", requiredBytes))
		}
		logger.Info("Disk space check passed", zap.Uint64("free", freeSpace))
	}

	snapshotPath = originalVolume
	if !cfg.SkipSnapshotCreation {
		snapshotName := fmt.Sprintf("snap-%d", time.Now().Unix())
		err = lvm.CreateSnapshot(originalVolume, snapshotName, cfg.SnapshotSize)
		if err != nil {
			logger.Fatal("Snapshot creation failed", zap.Error(err))
		}
		snapshotPath = lvm.GetSnapshotDevicePath(snapshotName, cfg.VolumeGroup)
		logger.Info("Snapshot created", zap.String("snapshot", snapshotPath))

		stopMonitor := make(chan struct{})
		go func() {
			if err := lvm.MonitorSnapshot(snapshotPath, 80.0, 10*time.Second, stopMonitor); err != nil {
				zap.L().Error("Snapshot monitor error", zap.Error(err))
				os.Exit(1)
			}
		}()
		defer close(stopMonitor)
	}

	err = transfer.DumpChangesSequential(snapshotPath, originalVolume, os.Stdout, cfg.Verbose > 0,
		cfg.ZeroCopy, cfg.VerifyChecksum, cfg.Compress, cfg.CompressLevel, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
	if err != nil {
		logger.Fatal("Copy operation failed", zap.Error(err))
	}

	if !cfg.SkipSnapshotCreation {
		err = lvm.RemoveSnapshot(snapshotPath)
		if err != nil {
			logger.Warn("Failed to remove snapshot", zap.Error(err))
		} else {
			logger.Info("Snapshot removed", zap.String("snapshot", snapshotPath))
		}
	}
}
