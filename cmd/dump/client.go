package dump

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"lvmsync_go/common"
	"lvmsync_go/config"
	"lvmsync_go/device"
	"lvmsync_go/remote"
	"lvmsync_go/transfer"
	"lvmsync_go/transport"
	_ "lvmsync_go/transport/h2"
	_ "lvmsync_go/transport/quic"
	_ "lvmsync_go/transport/ssh"
	_ "lvmsync_go/transport/tcp_tls"
)

// ErrRemoteCommand indicates that execution of the remote command failed.
var ErrRemoteCommand = errors.New("remote command error")

var (
	dumpChangesSequential = func(t *transfer.Transfer, cfg *config.Config, snap, origin string, out io.Writer) error {
		return t.DumpChangesSequential(cfg, snap, origin, out)
	}
	dumpChangesParallel = func(t *transfer.Transfer, cfg *config.Config, snap, origin string, out io.Writer) error {
		return t.DumpChangesParallel(cfg, snap, origin, out)
	}
	dumpChangesWithDeduplication = func(t *transfer.Transfer, cfg *config.Config, snap, origin string, out io.Writer, d transfer.DeduplicationStrategy) error {
		return t.DumpChangesWithDeduplication(cfg, snap, origin, out, d)
	}
	newSSHClient = remote.NewSSHClient
	openFile     = os.OpenFile
)

var copyBufferPool = sync.Pool{New: func() any {
	buf := make([]byte, 32*1024)
	return &buf
}}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *contextReader) Read(p []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
		return c.r.Read(p)
	}
}

// CopyPipeAsync copies data from src to dst in a new goroutine and returns a channel with the result.
func CopyPipeAsync(ctx context.Context, dst io.Writer, src io.Reader) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		bufAny := copyBufferPool.Get()
		buf := *(bufAny.(*[]byte))
		defer copyBufferPool.Put(&buf)

		_, err := io.CopyBuffer(dst, &contextReader{ctx: ctx, r: src}, buf)
		errCh <- err
	}()
	return errCh
}

// ExecuteDump selects the appropriate dump implementation based on configuration.
func ExecuteDump(cfg *config.Config, snapshotDevice, originDevice string, out io.Writer, logger *zap.Logger) error {
	t := transfer.NewTransfer(logger, &sync.WaitGroup{})
	dedup := transfer.NewDeduplicationStrategy(cfg, logger)
	if dedup != nil {
		defer func() {
			if err := dedup.SaveState(); err != nil {
				logger.Error("Failed to save dedup state", zap.Error(err))
			}
		}()
		return dumpChangesWithDeduplication(t, cfg, snapshotDevice, originDevice, out, dedup)
	}
	if cfg.Parallel <= 1 {
		return dumpChangesSequential(t, cfg, snapshotDevice, originDevice, out)
	}
	return dumpChangesParallel(t, cfg, snapshotDevice, originDevice, out)
}

// Run executes client mode transferring data to dest.
func Run(ctx context.Context, cfg *config.Config, snapshotDevice, dest string, logger *zap.Logger) error {
	originDevice := snapshotDevice
	if cfg.StdoutMode {
		limitedOut := transfer.WrapRateLimitedWriter(os.Stdout, cfg.SpeedLimit)
		return ExecuteDump(cfg, snapshotDevice, originDevice, limitedOut, logger)
	}
	if strings.Contains(dest, ":") {
		return RunRemoteDump(ctx, cfg, snapshotDevice, originDevice, dest, logger)
	}
	return RunLocalDump(cfg, snapshotDevice, originDevice, dest, logger)
}

// RunLocalDump dumps changes to a local destination device.
func RunLocalDump(cfg *config.Config, snapshotDevice, originDevice, dest string, logger *zap.Logger) (err error) {
	if cfg.DestType == "auto" {
		if dev, err := device.Detect(dest); err == nil {
			switch dev.(type) {
			case *device.RawDevice:
				if !cfg.SkipSnapshotCreation {
					dev.Close()
					return fmt.Errorf("raw destinations require --skip_snapshot_creation or external freeze hooks")
				}
				cfg.DestType = "raw"
			case *device.LVMDevice:
				cfg.DestType = "lvm"
			case *device.FileDevice:
				cfg.DestType = "file"
			}
			dev.Close()
		}
	}
	destFile, err := openFile(dest, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open destination device %s: %w", dest, err)
	}
	defer common.CloseWithErr(destFile, &err, "close destination device")
	limitedOut := transfer.WrapRateLimitedWriter(destFile, cfg.SpeedLimit)
	return ExecuteDump(cfg, snapshotDevice, originDevice, limitedOut, logger)
}

// SetupSSHClient creates an SSH client for remote operations.
func SetupSSHClient(ctx context.Context, cfg *config.Config, destHost string, logger *zap.Logger) (*remote.SSHClient, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.SSHTimeout)
	client, err := newSSHClient(ctx, destHost, cfg.SSHUser, cfg.SSHKeyPath, cfg.SSHPort, cfg.KnownHosts, cfg.StrictHostKeyCheck, cfg.SSHTimeout, cfg.SSHKeepAliveInterval, cfg.MaxRetries, logger)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create SSH client: %w", err)
	}
	return client, cancel, nil
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

// SetupSessionStreams wires local stdio to the remote session.
func SetupSessionStreams(ctx context.Context, session pipeSession) (io.WriteCloser, <-chan error, <-chan error, error) {
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}
	stdoutErrCh := CopyPipeAsync(ctx, os.Stdout, stdoutPipe)
	stderrErrCh := CopyPipeAsync(ctx, os.Stderr, stderrPipe)

	remoteStdin, err := session.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get remote stdin: %w", err)
	}

	return remoteStdin, stdoutErrCh, stderrErrCh, nil
}

// StreamToRemote dumps snapshot data to the remote stdin and closes the stream.
func StreamToRemote(cfg *config.Config, remoteStdin io.WriteCloser, snapshotDevice, originDevice string, logger *zap.Logger) error {
	streamErr := ExecuteDump(cfg, snapshotDevice, originDevice, remoteStdin, logger)

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

// WaitForRemoteCompletion waits for the remote command and I/O copies to finish.
func WaitForRemoteCompletion(session waitSession, stdoutErrCh, stderrErrCh <-chan error) error {
	if err := session.Wait(); err != nil {
		return fmt.Errorf("%w: %v", ErrRemoteCommand, err)
	}
	if err := <-stdoutErrCh; err != nil {
		return fmt.Errorf("stdout copy error: %w", err)
	}
	if err := <-stderrErrCh; err != nil {
		return fmt.Errorf("stderr copy error: %w", err)
	}
	return nil
}

// ExecuteRemoteCommand runs the remote apply command over SSH.
func ExecuteRemoteCommand(ctx context.Context, cfg *config.Config, client *remote.SSHClient, destDevice, snapshotDevice, originDevice string, logger *zap.Logger) (err error) {
	if err = client.ValidateRemoteCommand(ctx, cfg.LVMSyncPath); err != nil {
		return fmt.Errorf("remote command validation failed: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer closeSession(session, &err)

	remoteStdin, stdoutErrCh, stderrErrCh, err := SetupSessionStreams(ctx, session)
	if err != nil {
		return err
	}

	remoteCmd := fmt.Sprintf("%s --apply - %s", cfg.LVMSyncPath, destDevice)
	logger.Info("Starting remote apply command", zap.String("command", remoteCmd))

	if err = session.Start(remoteCmd); err != nil {
		return fmt.Errorf("failed to start remote command: %w", err)
	}

	if err = StreamToRemote(cfg, remoteStdin, snapshotDevice, originDevice, logger); err != nil {
		return err
	}

	return WaitForRemoteCompletion(session, stdoutErrCh, stderrErrCh)
}

// RunRemoteDump streams snapshot data to a remote host over SSH.
func RunRemoteDump(ctx context.Context, cfg *config.Config, snapshotDevice, originDevice, dest string, logger *zap.Logger) (err error) {
	parts := strings.SplitN(dest, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid destination %q: expected host:device", dest)
	}
	destHost, destDevice := parts[0], parts[1]
	client, cancel, err := SetupSSHClient(ctx, cfg, destHost, logger)
	if err != nil {
		return err
	}
	defer func() {
		cancel()
		if err2 := client.Close(); err2 != nil && !errors.Is(err2, io.EOF) {
			if err == nil {
				err = fmt.Errorf("failed to close SSH client: %w", err2)
			} else {
				err = fmt.Errorf("%v; failed to close SSH client: %w", err, err2)
			}
		}
	}()

	if cfg.RemotePreScript != "" {
		scriptCtx, cancel := context.WithTimeout(ctx, cfg.SSHTimeout)
		if err = client.RunRemoteScript(scriptCtx, cfg.RemotePreScript); err != nil {
			cancel()
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("remote pre-script context error: %w", err)
			}
			return fmt.Errorf("remote pre-script failed: %w", err)
		}
		cancel()
	}
	if cfg.RemotePostScript != "" {
		defer func() {
			scriptCtx, cancel := context.WithTimeout(ctx, cfg.SSHTimeout)
			defer cancel()
			if err2 := client.RunRemoteScript(scriptCtx, cfg.RemotePostScript); err2 != nil {
				msg := "remote post-script failed"
				if errors.Is(err2, context.Canceled) || errors.Is(err2, context.DeadlineExceeded) {
					msg = "remote post-script context error"
				}
				if err == nil {
					err = fmt.Errorf("%s: %w", msg, err2)
				} else {
					err = fmt.Errorf("%v; %s: %w", err, msg, err2)
				}
			}
		}()
	}

	validationCtx, cancel := context.WithTimeout(ctx, cfg.SSHTimeout)
	defer cancel()
	return ExecuteRemoteCommand(validationCtx, cfg, client, destDevice, snapshotDevice, originDevice, logger)
}

// SelectTransport chooses the first supported transport from cfg.Transport and
// logs the selected implementation. Unknown transports are skipped with a
// warning and an error is returned if none are supported.
func SelectTransport(cfg *config.Config, logger *zap.Logger) (transport.Interface, error) {
	if cfg.Transport == "" {
		return nil, nil
	}
	for _, name := range strings.Split(cfg.Transport, ",") {
		name = strings.TrimSpace(name)
		tr, err := transport.Get(name, transport.Config{Logger: logger})
		if err != nil {
			trLogger.Warn("unsupported transport", zap.String("transport", name))
			continue
		}
		trLogger.Info("selected transport", zap.String("transport", name))
		return tr, nil
	}
	return nil, fmt.Errorf("no supported transports: %s", cfg.Transport)
}
