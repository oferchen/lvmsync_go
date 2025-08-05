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

// copyPipeAsync copies data from src to dst in a new goroutine and returns a channel that receives
// the resulting error from io.Copy.
func copyPipeAsync(dst io.Writer, src io.Reader) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(dst, src)
		errCh <- err
	}()
	return errCh
}

// syncLogger flushes any buffered log entries and logs if the sync fails.
func syncLogger(logger *zap.Logger) {
	if err := logger.Sync(); err != nil {
		logger.Error("Logger sync error", zap.Error(err))
	}
}

var (
	cfg                          *config.Config
	applyFunc                    = transfer.RunApply
	dumpChangesSequential        = transfer.DumpChangesSequential
	dumpChangesParallel          = transfer.DumpChangesParallel
	dumpChangesWithDeduplication = transfer.DumpChangesWithDeduplication
)

func runApplyMode(applyFile string) error {
	args := pflag.Args()
	if len(args) < 1 {
		return fmt.Errorf("no destination device specified for apply mode")
	}
	destDevice := args[0]

	return applyFunc(cfg, applyFile, destDevice)
}

func executeDump(cfg *config.Config, snapshotDevice, originDevice string, out io.Writer) error {
	if cfg.Deduplication {
		dedup := transfer.NewDeduplicationStrategy(cfg)
		defer func() {
			if err := dedup.SaveState(); err != nil {
				zap.L().Error("Failed to save dedup state", zap.Error(err))
			}
		}()
		return dumpChangesWithDeduplication(cfg, snapshotDevice, originDevice, out, dedup)
	}
	if cfg.Parallel <= 1 {
		return dumpChangesSequential(cfg, snapshotDevice, originDevice, out)
	}
	return dumpChangesParallel(cfg, snapshotDevice, originDevice, out)
}

func runClientMode(snapshotDevice, dest string) (err error) {
	originDevice := snapshotDevice
	if cfg.StdoutMode {
		limitedOut := transfer.WrapRateLimitedWriter(os.Stdout, cfg.SpeedLimit)
		return executeDump(cfg, snapshotDevice, originDevice, limitedOut)
	}
	if strings.Contains(dest, ":") {
		parts := strings.SplitN(dest, ":", 2)
		destHost := parts[0]
		destDevice := parts[1]
		client, err := remote.NewSSHClient(destHost, cfg.SSHUser, cfg.SSHKeyPath, cfg.SSHPort, cfg.KnownHosts, cfg.StrictHostKeyCheck, cfg.SSHTimeout, cfg.SSHKeepAliveInterval, cfg.MaxRetries)
		if err != nil {
			return fmt.Errorf("failed to create SSH client: %w", err)
		}
		defer client.Close()
		remote.SetLogger(zap.L())

		if cfg.RemotePreScript != "" {
			if err := remote.RunRemoteScript(client, cfg.RemotePreScript); err != nil {
				return fmt.Errorf("remote pre-script failed: %w", err)
			}
		}
		if cfg.RemotePostScript != "" {
			defer func() {
				if err2 := remote.RunRemoteScript(client, cfg.RemotePostScript); err2 != nil {
					if err == nil {
						err = fmt.Errorf("remote post-script failed: %w", err2)
					} else {
						err = fmt.Errorf("%v; remote post-script failed: %w", err, err2)
					}
				}
			}()
		}
		if err := remote.ValidateRemoteCommand(client, cfg.LVMSyncPath); err != nil {
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
		stdoutErrCh := copyPipeAsync(os.Stdout, stdoutPipe)
		stderrErrCh := copyPipeAsync(os.Stderr, stderrPipe)

		remoteCmd := fmt.Sprintf("%s --apply - %s", cfg.LVMSyncPath, destDevice)
		zap.L().Info("Starting remote apply command", zap.String("command", remoteCmd))

		remoteStdin, err := session.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to get remote stdin: %w", err)
		}
		if err := session.Start(remoteCmd); err != nil {
			return fmt.Errorf("failed to start remote command: %w", err)
		}

		streamErr := executeDump(cfg, snapshotDevice, originDevice, remoteStdin)

		remoteStdin.Close()
		if streamErr != nil {
			return fmt.Errorf("error during dumpChanges: %w", streamErr)
		}

		if err := session.Wait(); err != nil {
			return fmt.Errorf("remote command error: %w", err)
		}

		if err := <-stdoutErrCh; err != nil {
			return fmt.Errorf("stdout copy error: %w", err)
		}
		if err := <-stderrErrCh; err != nil {
			return fmt.Errorf("stderr copy error: %w", err)
		}
		return nil
	}
	destFile, err := os.OpenFile(dest, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open destination device %s: %w", dest, err)
	}
	defer destFile.Close()

	limitedOut := transfer.WrapRateLimitedWriter(destFile, cfg.SpeedLimit)
	return executeDump(cfg, snapshotDevice, originDevice, limitedOut)
}

func handleSignals(signals <-chan os.Signal, snapshotPath *string) {
	sig := <-signals
	zap.L().Info("Received signal, aborting", zap.String("signal", sig.String()))
	if !cfg.SkipSnapshotCreation && *snapshotPath != "" && *snapshotPath != pflag.Arg(0) {
		if err := lvm.RemoveSnapshot(*snapshotPath); err != nil {
			zap.L().Warn("Failed to remove snapshot on shutdown", zap.Error(err))
		} else {
			zap.L().Info("Snapshot removed on shutdown", zap.String("snapshot", *snapshotPath))
		}
	}
	os.Exit(1)
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
	defer syncLogger(logger)

	logger.Info("Effective configuration",
		zap.String("block_size", cfg.HumanBlockSize()),
		zap.Int("parallel", cfg.Parallel),
		zap.Bool("deduplication", cfg.Deduplication),
		zap.String("dedup_strategy", cfg.DedupStrategy),
		zap.String("compress", cfg.Compress),
		zap.Int("compress_level", cfg.CompressLevel),
		zap.Int("compress_concurrency", cfg.CompressConcurrency),
		zap.String("snapshot_size", cfg.SnapshotSize),
		zap.String("volume_group", cfg.VolumeGroup),
		zap.String("target_volume_group", cfg.TargetVolumeGroup),
		zap.Bool("stdout_mode", cfg.StdoutMode),
		zap.String("lvmsync_path", cfg.LVMSyncPath),
	)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	var snapshotPath string

	go handleSignals(signals, &snapshotPath)

	if cfg.ApplyMode != "" {
		if err := runApplyMode(cfg.ApplyMode); err != nil {
			logger.Fatal("Apply operation failed", zap.Error(err))
		}
		return
	}

	args := pflag.Args()
	if (cfg.StdoutMode && len(args) < 1) || (!cfg.StdoutMode && len(args) < 2) {
		pflag.Usage()
		os.Exit(1)
	}
	originalVolume := args[0]
	destPath := ""
	if !cfg.StdoutMode {
		destPath = args[1]
	}

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

	if err := runClientMode(snapshotPath, destPath); err != nil {
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
