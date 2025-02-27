// main.go
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"lvmsync_go/config"
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
		client, err := remote.NewSSHClient(destHost, cfg.SSHUser, cfg.SSHKeyPath, cfg.SSHPort, cfg.KnownHosts, cfg.SSHVerify)
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
			streamErr = transfer.DumpChangesSequential(snapshotDevice, originDevice, remoteStdin, cfg.Verbose > 0, cfg.ZeroCopy, cfg.VerifyChecksum, cfg.Compress, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
		} else {
			streamErr = transfer.DumpChangesParallel(snapshotDevice, originDevice, remoteStdin, cfg.Verbose > 0, cfg.VerifyChecksum, cfg.Compress, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
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
			return transfer.DumpChangesSequential(snapshotDevice, originDevice, limitedOut, cfg.Verbose > 0, cfg.ZeroCopy, cfg.VerifyChecksum, cfg.Compress, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
		}
		return transfer.DumpChangesParallel(snapshotDevice, originDevice, limitedOut, cfg.Verbose > 0, cfg.VerifyChecksum, cfg.Compress, cfg.SpeedLimit, cfg.ResumeState, cfg.Parallel)
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
	verboseCount := pflag.Lookup("verbose").Value.String()
	var logger *zap.Logger
	if cfg.StdoutMode || cfg.Parallel <= 1 || verboseCount != "0" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()
	args := pflag.Args()
	if cfg.ApplyMode != "" {
		if len(args) < 1 {
			logger.Fatal("No destination device specified for apply mode")
		}
		if err := runApplyMode(); err != nil {
			logger.Fatal("Apply mode error", zap.Error(err))
		}
		return
	}
	if len(args) < 2 {
		pflag.Usage()
		logger.Fatal("Usage: lvmsync [options] <snapshot device> <desthost:destdevice OR destdevice>")
	}
	snapshotDevice := args[0]
	dest := args[1]
	if cfg.ZeroCopy && cfg.Parallel > 1 {
		logger.Warn("Zero-copy only works in sequential mode; disabling zerocopy")
		cfg.ZeroCopy = false
	}
	if err := runClientMode(snapshotDevice, dest); err != nil {
		logger.Fatal("Client mode error", zap.Error(err))
	}
}
