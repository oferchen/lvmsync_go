// main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	signalspkg "lvmsync_go/cmd/lvmsync/signals"
	"lvmsync_go/common"
	"lvmsync_go/config"
	grpcclient "lvmsync_go/grpc/client"
	grpcserver "lvmsync_go/grpc/server"
	clientpkg "lvmsync_go/internal/client"
	"lvmsync_go/internal/privesc"
	"lvmsync_go/internal/transport"
	"lvmsync_go/lvm"
	"lvmsync_go/proto"
	"lvmsync_go/remote"
	"lvmsync_go/transfer"
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
	newSSHClient                 = remote.NewSSHClient
	openFile                     = os.OpenFile
	runFunc                      = run
	exitFunc                     = os.Exit
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
	dedup := transfer.NewDeduplicationStrategy(cfg)
	if dedup != nil {
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
		return runRemoteDump(snapshotDevice, originDevice, dest)
	}
	return runLocalDump(snapshotDevice, originDevice, dest)
}

func runLocalDump(snapshotDevice, originDevice, dest string) (err error) {
	destFile, err := openFile(dest, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open destination device %s: %w", dest, err)
	}
	defer common.CloseWithErr(destFile, &err, "close destination device")
	limitedOut := transfer.WrapRateLimitedWriter(destFile, cfg.SpeedLimit)
	return executeDump(cfg, snapshotDevice, originDevice, limitedOut)
}

func setupSSHClient(destHost string) (*ssh.Client, error) {
	client, err := newSSHClient(destHost, cfg.SSHUser, cfg.SSHKeyPath, cfg.SSHPort, cfg.KnownHosts, cfg.StrictHostKeyCheck, cfg.SSHTimeout, cfg.SSHKeepAliveInterval, cfg.MaxRetries)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client: %w", err)
	}
	remote.SetLogger(zap.L())
	return client, nil
}

func closeSession(session *ssh.Session, errp *error) {
	if err2 := session.Close(); err2 != nil && !errors.Is(err2, io.EOF) {
		if *errp == nil {
			*errp = fmt.Errorf("failed to close SSH session: %w", err2)
		} else {
			*errp = fmt.Errorf("%v; failed to close SSH session: %w", *errp, err2)
		}
	}
}

type pipeSession interface {
	StdoutPipe() (io.Reader, error)
	StderrPipe() (io.Reader, error)
	StdinPipe() (io.WriteCloser, error)
}

func setupSessionStreams(session pipeSession) (io.WriteCloser, <-chan error, <-chan error, error) {
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}
	stdoutErrCh := copyPipeAsync(os.Stdout, stdoutPipe)
	stderrErrCh := copyPipeAsync(os.Stderr, stderrPipe)

	remoteStdin, err := session.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get remote stdin: %w", err)
	}

	return remoteStdin, stdoutErrCh, stderrErrCh, nil
}

func streamToRemote(remoteStdin io.WriteCloser, snapshotDevice, originDevice string) error {
	streamErr := executeDump(cfg, snapshotDevice, originDevice, remoteStdin)

	if err := remoteStdin.Close(); err != nil && !errors.Is(err, io.EOF) {
		if streamErr != nil {
			return fmt.Errorf("%v; failed to close remote stdin: %w", streamErr, err)
		}
		return fmt.Errorf("failed to close remote stdin: %w", err)
	}
	if streamErr != nil {
		return fmt.Errorf("error during dumpChanges: %w", streamErr)
	}

	return nil
}

type waitSession interface {
	Wait() error
}

func waitForRemoteCompletion(session waitSession, stdoutErrCh, stderrErrCh <-chan error) error {
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

//nolint:revive // high-level orchestration inherently complex
func executeRemoteCommand(client *ssh.Client, destDevice, snapshotDevice, originDevice string) (err error) {
	if err = remote.ValidateRemoteCommand(client, cfg.LVMSyncPath); err != nil {
		return fmt.Errorf("remote command validation failed: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer closeSession(session, &err)

	remoteStdin, stdoutErrCh, stderrErrCh, err := setupSessionStreams(session)
	if err != nil {
		return err
	}

	remoteCmd := fmt.Sprintf("%s --apply - %s", cfg.LVMSyncPath, destDevice)
	zap.L().Info("Starting remote apply command", zap.String("command", remoteCmd))

	if err = session.Start(remoteCmd); err != nil {
		return fmt.Errorf("failed to start remote command: %w", err)
	}

	if err = streamToRemote(remoteStdin, snapshotDevice, originDevice); err != nil {
		return err
	}

	return waitForRemoteCompletion(session, stdoutErrCh, stderrErrCh)
}

//nolint:revive // network orchestration requires complexity
func runRemoteDump(snapshotDevice, originDevice, dest string) (err error) {
	parts := strings.SplitN(dest, ":", 2)
	destHost, destDevice := parts[0], parts[1]
	client, err := setupSSHClient(destHost)
	if err != nil {
		return err
	}
	defer func() {
		if err2 := client.Close(); err2 != nil && !errors.Is(err2, io.EOF) {
			if err == nil {
				err = fmt.Errorf("failed to close SSH client: %w", err2)
			} else {
				err = fmt.Errorf("%v; failed to close SSH client: %w", err, err2)
			}
		}
	}()

	if cfg.RemotePreScript != "" {
		if err = remote.RunRemoteScript(client, cfg.RemotePreScript); err != nil {
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

	return executeRemoteCommand(client, destDevice, snapshotDevice, originDevice)
}

func configure() (*zap.Logger, error) {
	var err error
	cfg, err = config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	if err = privesc.EnsureRoot(cfg.LVMEscalation); err != nil {
		return nil, fmt.Errorf("privilege escalation error: %w", err)
	}

	if err = cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation error: %w", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("logger initialization error: %w", err)
	}
	zap.ReplaceGlobals(logger)

	logger.Info("Effective configuration",
		zap.String("block_size", cfg.HumanBlockSize()),
		zap.Int("parallel", cfg.Parallel),
		zap.String("dedup_strategy", cfg.DedupStrategy),
		zap.String("compress", cfg.Compress),
		zap.Int("compress_level", cfg.CompressLevel),
		zap.Int("compress_concurrency", cfg.CompressConcurrency),
		zap.String("snapshot_size", cfg.SnapshotSize),
		zap.String("volume_group", cfg.VolumeGroup),
		zap.String("target_volume_group", cfg.TargetVolumeGroup),
		zap.Bool("stdout_mode", cfg.StdoutMode),
		zap.String("lvmsync_path", cfg.LVMSyncPath),
		zap.String("grpc_listen", cfg.GRPCListen),
		zap.String("grpc_connect", cfg.GRPCConnect),
	)

	return logger, nil
}

func run() error {
	logger, err := configure()
	if err != nil {
		return err
	}
	defer syncLogger(logger)
	defer lvm.Cleanup()

	if cfg.Transport != "" {
		order := strings.Split(cfg.Transport, ",")
		_, _, name, err := transport.Select(cfg, order)
		if err != nil {
			return err
		}
		logger.Info("selected transport", zap.String("transport", name))
	}

	if cfg.GRPCListen != "" {
		srvCfg := grpcserver.Config{TLSCert: cfg.TLSCert, TLSKey: cfg.TLSKey, CACert: cfg.CACert, AllowInsecure: cfg.AllowInsecure}
		ln, err := net.Listen("tcp", cfg.GRPCListen)
		if err != nil {
			return fmt.Errorf("gRPC listen: %w", err)
		}
		srv, err := grpcserver.New(srvCfg, nil)
		if err != nil {
			return fmt.Errorf("gRPC server: %w", err)
		}
		go func() {
			if err := srv.Serve(ln); err != nil {
				zap.L().Error("grpc server", zap.Error(err))
			}
		}()
		defer srv.GracefulStop()
	}

	if cfg.GRPCConnect != "" {
		conn, err := grpcclient.Dial(cfg.GRPCConnect, grpcclient.Config{
			TLSCert:       cfg.TLSCert,
			TLSKey:        cfg.TLSKey,
			CACert:        cfg.CACert,
			AllowInsecure: cfg.AllowInsecure,
		})
		if err != nil {
			return fmt.Errorf("gRPC dial: %w", err)
		}
		c := proto.NewReplicationClient(conn)
		hs := &proto.HandshakeRequest{SectorSize: 512, Alignment: 512, MaxConcurrency: uint32(cfg.Parallel), DedupSupported: true, CompressionSupported: true}
		if _, err := grpcclient.Handshake(context.Background(), c, hs); err != nil {
			conn.Close()
			return fmt.Errorf("handshake failed: %w", err)
		}
		sess, err := grpcclient.CreateSession(context.Background(), c, "vol", "dev")
		if err != nil {
			conn.Close()
			return fmt.Errorf("create session: %w", err)
		}
		stream, err := grpcclient.AckStream(context.Background(), c, sess.GetSessionId())
		if err != nil {
			conn.Close()
			return fmt.Errorf("ack stream: %w", err)
		}
		go func(id string) {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if err := stream.Send(&proto.Ack{SessionId: id, Ok: true, Message: "ping"}); err != nil {
					return
				}
			}
		}(sess.GetSessionId())
		defer stream.CloseSend()
		defer conn.Close()
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	var snapshotPath string
	sigErrCh := make(chan error, 1)
	go signalspkg.Handle(cfg, signals, &snapshotPath, sigErrCh)

	if cfg.ApplyMode != "" {
		if err = runApplyMode(cfg.ApplyMode); err != nil {
			return fmt.Errorf("apply operation failed: %w", err)
		}
		return nil
	}

	args := pflag.Args()
	if (cfg.StdoutMode && len(args) < 1) || (!cfg.StdoutMode && len(args) < 2) {
		pflag.Usage()
		return fmt.Errorf("invalid arguments")
	}
	originalVolume := args[0]
	destPath := ""
	if !cfg.StdoutMode {
		destPath = args[1]
	}

	var monitorErrCh chan error
	snapshotPath, monitorErrCh, cleanup, err := clientpkg.PrepareSnapshot(cfg, originalVolume, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	return clientpkg.ExecuteClient(runClientMode, snapshotPath, destPath, sigErrCh, monitorErrCh)
}

func main() {
	if err := runFunc(); err != nil {
		logger := zap.L()
		logger.Error("run failed", zap.Error(err))
		syncLogger(logger)
		exitFunc(1)
	}
}
