package dump

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/common"
	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	cpufeatures "lvmsync_go/internal/cpufeatures"
	digestpkg "lvmsync_go/internal/digest"
	"lvmsync_go/internal/privilege"
	"lvmsync_go/internal/rsyncwire"
	"lvmsync_go/lvm"
	"lvmsync_go/remote"
	"lvmsync_go/transfer"
	"lvmsync_go/transport"
	_ "lvmsync_go/transport/h2"
	_ "lvmsync_go/transport/quic"
	_ "lvmsync_go/transport/rsyncwire"
	_ "lvmsync_go/transport/ssh"
	_ "lvmsync_go/transport/tcp_tls"
)

// ErrRemoteCommand indicates that execution of the remote command failed.
var ErrRemoteCommand = errors.New("remote command error")

const maxFrame = 1 << 20

func chooseCompression(chunkLen int, compress string) string {
	if compress != transfer.StrategyAuto && compress != "" {
		return compress
	}
	if chunkLen > 0 && chunkLen < 256*1024 {
		return "lz4"
	}
	if cpufeatures.HasAVX2() || cpufeatures.HasNEON() {
		return "zstd"
	}
	return "lz4"
}

// Runner manages external interactions for dump operations.
type Runner struct {
	dumpSeq        func(context.Context, *transfer.Transfer, *config.Config, string, string, io.Writer) error
	dumpPar        func(context.Context, *transfer.Transfer, *config.Config, string, string, io.Writer) error
	dumpDedup      func(context.Context, *transfer.Transfer, *config.Config, string, string, io.Writer, transfer.DeduplicationStrategy) error
	newSSHClient   func(context.Context, string, string, string, int, string, bool, time.Duration, time.Duration, int, *zap.Logger) (*remote.SSHClient, error)
	openFile       func(string, int, os.FileMode) (*os.File, error)
	detectDevice   func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error)
	sumFile        func(string, string) ([32]byte, error)
	streamToRemote func(context.Context, *config.Config, io.WriteCloser, string, string, string, *zap.Logger) error
	probeDest      func(context.Context, *config.Config, string, *zap.Logger) (device.DeviceIdentity, error)
	createLV       func(context.Context, string, string, uint64, *zap.Logger) error
	parseLVPath    func(string) (string, string, error)
	getVolumeSize  func(string, *lvm.FDCache, *zap.Logger) (uint64, error)
	newFDC         func(*zap.Logger) (*lvm.FDCache, error)
}

var (
	openFile       = os.OpenFile
	sumFile        = digestpkg.SumFile
	newSSHClient   = remote.NewSSHClient
	detectDevice   = device.Detect
	streamToRemote = func(ctx context.Context, cfg *config.Config, remoteStdin io.WriteCloser, snapshotDevice, originDevice, alg string, logger *zap.Logger) error {
		r := &Runner{
			dumpSeq:   dumpChangesSequential,
			dumpPar:   dumpChangesParallel,
			dumpDedup: dumpChangesWithDeduplication,
			sumFile:   sumFile,
		}
		return r.StreamToRemote(ctx, cfg, remoteStdin, snapshotDevice, originDevice, alg, logger)
	}
	dumpChangesSequential = func(ctx context.Context, t *transfer.Transfer, cfg *config.Config, snap, origin string, out io.Writer) error {
		return t.DumpChangesSequential(ctx, cfg, snap, origin, out)
	}
	dumpChangesParallel = func(ctx context.Context, t *transfer.Transfer, cfg *config.Config, snap, origin string, out io.Writer) error {
		return t.DumpChangesParallel(ctx, cfg, snap, origin, out)
	}
	dumpChangesWithDeduplication = func(ctx context.Context, t *transfer.Transfer, cfg *config.Config, snap, origin string, out io.Writer, d transfer.DeduplicationStrategy) error {
		return t.DumpChangesWithDeduplication(ctx, cfg, snap, origin, out, d)
	}
	probeDestination = func(ctx context.Context, cfg *config.Config, dest string, logger *zap.Logger) (device.DeviceIdentity, error) {
		return realProbeDestination(ctx, cfg, dest, logger)
	}
)

// NewRunner constructs a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{
		dumpSeq:        dumpChangesSequential,
		dumpPar:        dumpChangesParallel,
		dumpDedup:      dumpChangesWithDeduplication,
		newSSHClient:   newSSHClient,
		openFile:       openFile,
		detectDevice:   detectDevice,
		sumFile:        sumFile,
		streamToRemote: streamToRemote,
		probeDest:      probeDestination,
		createLV:       lvm.CreateLogicalVolume,
		parseLVPath:    lvm.ParseLVPath,
		getVolumeSize:  lvm.GetVolumeSize,
		newFDC:         lvm.NewDeviceFDCache,
	}
}

// ExecuteDump is a convenience wrapper using a default Runner.
func ExecuteDump(ctx context.Context, cfg *config.Config, snapshotDevice, originDevice string, out io.Writer, logger *zap.Logger) error {
	return NewRunner().ExecuteDump(ctx, cfg, snapshotDevice, originDevice, out, logger)
}

// NewRunnerWithDeps constructs a Runner overriding defaults.
func NewRunnerWithDeps(deps *Runner) *Runner {
	r := NewRunner()
	if deps == nil {
		return r
	}
	if deps.dumpSeq != nil {
		r.dumpSeq = deps.dumpSeq
	}
	if deps.dumpPar != nil {
		r.dumpPar = deps.dumpPar
	}
	if deps.dumpDedup != nil {
		r.dumpDedup = deps.dumpDedup
	}
	if deps.newSSHClient != nil {
		r.newSSHClient = deps.newSSHClient
	}
	if deps.openFile != nil {
		r.openFile = deps.openFile
	}
	if deps.detectDevice != nil {
		r.detectDevice = deps.detectDevice
	}
	if deps.sumFile != nil {
		r.sumFile = deps.sumFile
	}
	if deps.streamToRemote != nil {
		r.streamToRemote = deps.streamToRemote
	}
	if deps.probeDest != nil {
		r.probeDest = deps.probeDest
	}
	if deps.createLV != nil {
		r.createLV = deps.createLV
	}
	if deps.parseLVPath != nil {
		r.parseLVPath = deps.parseLVPath
	}
	if deps.getVolumeSize != nil {
		r.getVolumeSize = deps.getVolumeSize
	}
	if deps.newFDC != nil {
		r.newFDC = deps.newFDC
	}
	return r
}

// Run executes client mode transferring data to dest using a default Runner.
func Run(ctx context.Context, cfg *config.Config, source, dest string, logger *zap.Logger) (string, error) {
	return NewRunner().Run(ctx, cfg, source, dest, logger)
}

// RunLocalDump dumps changes to a local destination using a default Runner.
func RunLocalDump(ctx context.Context, cfg *config.Config, snapshotDevice, originDevice, dest string, logger *zap.Logger) (string, error) {
	return NewRunner().RunLocalDump(ctx, cfg, snapshotDevice, originDevice, dest, logger)
}

// RunRemoteDump streams snapshot data to a remote host using a default Runner.
func RunRemoteDump(ctx context.Context, cfg *config.Config, snapshotDevice, originDevice, dest string, logger *zap.Logger) error {
	return NewRunner().RunRemoteDump(ctx, cfg, snapshotDevice, originDevice, dest, logger)
}

// SetupSSHClient creates an SSH client using a default Runner.
func SetupSSHClient(ctx context.Context, cfg *config.Config, destHost string, logger *zap.Logger) (*remote.SSHClient, context.CancelFunc, error) {
	return NewRunner().SetupSSHClient(ctx, cfg, destHost, logger)
}

// StreamToRemote dumps snapshot data to a remote stdin using a default Runner.
func StreamToRemote(ctx context.Context, cfg *config.Config, remoteStdin io.WriteCloser, snapshotDevice, originDevice, alg string, logger *zap.Logger) error {
	return NewRunner().StreamToRemote(ctx, cfg, remoteStdin, snapshotDevice, originDevice, alg, logger)
}

func init() {
	r := NewRunner()
	rootcmd.RegisterDump(r.Run)
	rootcmd.RegisterSelectTransport(SelectTransport)
}

var copyBufferPool = sync.Pool{New: func() any {
	buf := make([]byte, 32*1024)
	return &buf
}}

type writeOnlyReadWriter struct{ io.Writer }

func (writeOnlyReadWriter) Read(p []byte) (int, error) { return 0, io.EOF }

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
		cr := &contextReader{ctx: ctx, r: src}
		for {
			if err := ctx.Err(); err != nil {
				errCh <- err
				return
			}
			n, err := cr.Read(buf)
			if n > 0 {
				written := 0
				for written < n {
					if err := ctx.Err(); err != nil {
						errCh <- err
						return
					}
					w, werr := dst.Write(buf[written:n])
					if w > 0 {
						written += w
					}
					if werr != nil {
						errCh <- werr
						return
					}
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					errCh <- nil
				} else {
					errCh <- err
				}
				return
			}
		}
	}()
	return errCh
}

// ExecuteDump selects the appropriate dump implementation based on configuration.
func (r *Runner) ExecuteDump(ctx context.Context, cfg *config.Config, snapshotDevice, originDevice string, out io.Writer, logger *zap.Logger) error {
	t := transfer.NewTransfer(logger, &sync.WaitGroup{}, nil)
	dedup := transfer.NewDeduplicationStrategy(cfg, logger)
	if dedup != nil {
		defer func() {
			if err := dedup.SaveState(); err != nil {
				logger.Error("Failed to save dedup state", zap.Error(err))
			}
		}()
		return r.dumpDedup(ctx, t, cfg, snapshotDevice, originDevice, out, dedup)
	}
	if cfg.Parallel <= 1 {
		return r.dumpSeq(ctx, t, cfg, snapshotDevice, originDevice, out)
	}
	return r.dumpPar(ctx, t, cfg, snapshotDevice, originDevice, out)
}

// Run executes client mode transferring data to dest and returns the destination type.
func (r *Runner) Run(ctx context.Context, cfg *config.Config, source, dest string, logger *zap.Logger) (string, error) {
	defer rootcmd.SyncLogger(logger)
	if cfg.ProbeOnly {
		id, err := r.probeDest(ctx, cfg, dest, logger)
		if err != nil {
			return cfg.DestType, err
		}
		fmt.Fprintf(os.Stdout, "%d %s %s %s %s %d %d %d\n", id.SizeBytes, id.KernelUUID, id.GPTUUID, id.MBRSignature, id.FSUUID, id.Major, id.Minor, id.ManifestEpoch)
		return cfg.DestType, nil
	}
	dev, err := r.detectDevice(
		ctx,
		source,
		cfg.Offline,
		cfg.SourceType,
		cfg.FSFreezeCommand,
		cfg.FSThawCommand,
		cfg.LVMEscalation,
		cfg.FreezeTimeout,
		cfg.ThawTimeout,
		privilege.New(ctx, logger),
		logger,
		device.NewRunner(),
	)
	if err != nil {
		return cfg.DestType, err
	}
	switch dev.(type) {
	case *device.LVMDevice:
		cfg.SourceType = "lvm"
	case *device.RawDevice:
		cfg.SourceType = "raw"
	case *device.FileDevice:
		cfg.SourceType = "file"
	}
	if cfg.DryRun {
		size := int64(dev.SizeBytes())
		durMs, bwBps := transfer.Estimate(size, cfg.SpeedLimit)
		algo := chooseCompression(cfg.BlockSize, cfg.Compress)
		logger.Info("dry run",
			zap.Int64("size_bytes", size),
			zap.Int64("estimated_duration_ms", durMs),
			zap.Int64("estimated_bandwidth_bps", bwBps),
			zap.String("compression", algo),
		)
		dev.Cleanup(ctx)
		dev.Close()
		return cfg.DestType, nil
	}
	if cfg.SourceType == "raw" && !cfg.SkipSnapshotCreation {
		dev.Close()
		dev.Cleanup(ctx)
		return cfg.DestType, fmt.Errorf("raw sources require --skip-snapshot-creation and either --offline or --fs-freeze-command/--fs-thaw-command")
	}
	snapDev, err := dev.Snapshot(ctx, cfg.SnapshotSize)
	if err != nil {
		dev.Cleanup(ctx)
		dev.Close()
		return cfg.DestType, err
	}
	snapshotDevice := snapDev.Path()
	originDevice := dev.Path()
	lvm.RegisterSnapshot(snapshotDevice, logger)
	defer lvm.CleanupSnapshot(ctx, snapshotDevice, logger)
	defer func() {
		snapDev.Close()
		if snapDev != dev {
			dev.Cleanup(ctx)
			dev.Close()
		}
	}()
	if cfg.StdoutMode {
		limitedOut := transfer.WrapRateLimitedWriter(os.Stdout, cfg.SpeedLimit)
		return cfg.DestType, r.ExecuteDump(ctx, cfg, snapshotDevice, originDevice, limitedOut, logger)
	}
	if strings.Contains(dest, ":") {
		return cfg.DestType, r.RunRemoteDump(ctx, cfg, snapshotDevice, originDevice, dest, logger)
	}
	return r.RunLocalDump(ctx, cfg, snapshotDevice, originDevice, dest, logger)
}

// RunLocalDump dumps changes to a local destination device and returns the detected destination type.
func (r *Runner) RunLocalDump(ctx context.Context, cfg *config.Config, snapshotDevice, originDevice, dest string, logger *zap.Logger) (string, error) {
	destType := cfg.DestType
	devCtx := device.WithForce(context.Background(), cfg.Force)
	devCtx = device.WithAllowOverwrite(devCtx, cfg.AllowOverwrite)
	devCtx = device.WithYesIKnow(devCtx, cfg.YesIKnow)
	devRunner := device.NewRunner()
	if cfg.DryRun {
		return destType, r.ExecuteDump(ctx, cfg, snapshotDevice, originDevice, io.Discard, logger)
	}
	if destType == "auto" {
		if dev, err := device.Detect(devCtx, dest, true, destType, "", "", cfg.LVMEscalation, cfg.FreezeTimeout, cfg.ThawTimeout, privilege.New(devCtx, logger), logger, devRunner); err == nil {
			switch dev.(type) {
			case *device.RawDevice:
				if !cfg.SkipSnapshotCreation {
					dev.Close()
					return destType, fmt.Errorf("raw destinations require --skip-snapshot-creation or external freeze hooks")
				}
				destType = "raw"
			case *device.LVMDevice:
				destType = "lvm"
			case *device.FileDevice:
				destType = "file"
			}
			dev.Close()
		}
	}
	if cfg.CreateDestLV {
		exists, err := lvm.VolumeExists(devCtx, dest)
		if err != nil {
			return destType, err
		}
		if !exists {
			if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
				vg, lv, err := r.parseLVPath(dest)
				if err != nil {
					return destType, err
				}
				cache, err := r.newFDC(logger)
				if err != nil {
					return destType, err
				}
				defer cache.Close()
				size, err := r.getVolumeSize(originDevice, cache, logger)
				if err != nil {
					return destType, err
				}
				if err := r.createLV(ctx, vg, lv, size, logger); err != nil {
					return destType, err
				}
			} else {
				cache, err := lvm.NewDeviceFDCache(logger)
				if err != nil {
					return destType, err
				}
				defer cache.Close()
				size, err := lvm.GetVolumeSize(snapshotDevice, cache, logger)
				if err != nil {
					return destType, err
				}
				if err := devRunner.CreateLV(devCtx, dest, size, cfg.LVMEscalation); err != nil {
					return destType, err
				}
			}
		}
	}
	destFile, err := r.openFile(dest, os.O_RDWR, 0)
	if err != nil {
		return destType, fmt.Errorf("failed to open destination device %s: %w", dest, err)
	}
	defer common.CloseWithErr(destFile, &err, "close destination device")
	limitedOut := transfer.WrapRateLimitedWriter(destFile, cfg.SpeedLimit)
	return destType, r.ExecuteDump(ctx, cfg, snapshotDevice, originDevice, limitedOut, logger)
}

// SetupSSHClient creates an SSH client for remote operations.
func (r *Runner) SetupSSHClient(ctx context.Context, cfg *config.Config, destHost string, logger *zap.Logger) (*remote.SSHClient, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.SSHTimeout)
	client, err := r.newSSHClient(ctx, destHost, cfg.SSHUser, cfg.SSHKeyPath, cfg.SSHPort, cfg.KnownHosts, cfg.StrictHostKeyCheck, cfg.SSHTimeout, cfg.SSHKeepAliveInterval, cfg.MaxRetries, logger)
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

// StreamToRemote dumps snapshot data to the remote stdin, then computes and
// transmits the digest before closing the stream.
func (r *Runner) StreamToRemote(ctx context.Context, cfg *config.Config, remoteStdin io.WriteCloser, snapshotDevice, originDevice, alg string, logger *zap.Logger) error {
	if cfg.Delta == "rsync" {
		return r.streamRsyncDelta(ctx, cfg, remoteStdin, snapshotDevice, originDevice, alg, logger)
	}

	streamErr := r.ExecuteDump(ctx, cfg, snapshotDevice, originDevice, remoteStdin, logger)
	if streamErr != nil {
		if err := remoteStdin.Close(); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("%v; failed to close remote stdin: %w", streamErr, err)
		}
		return fmt.Errorf("error during dumpChanges: %w", streamErr)
	}

	sum, err := r.sumFile(snapshotDevice, alg)
	if err != nil {
		remoteStdin.Close()
		return fmt.Errorf("compute digest: %w", err)
	}

	rw := writeOnlyReadWriter{remoteStdin}
	cl := rsyncwire.NewClient(rsyncwire.NewStream(rw, maxFrame))
	if err := cl.SendDigest(alg, sum); err != nil {
		remoteStdin.Close()
		return fmt.Errorf("send digest: %w", err)
	}

	if err := remoteStdin.Close(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to close remote stdin: %w", err)
	}
	return nil
}

func (r *Runner) streamRsyncDelta(ctx context.Context, cfg *config.Config, remoteStdin io.WriteCloser, snapshotDevice, originDevice, alg string, logger *zap.Logger) (err error) {
	snap, err := r.openFile(snapshotDevice, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer common.CloseWithErr(snap, &err, "close snapshot")

	orig, err := r.openFile(originDevice, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open origin: %w", err)
	}
	defer common.CloseWithErr(orig, &err, "close origin")

	rw := writeOnlyReadWriter{remoteStdin}
	cl := rsyncwire.NewClient(rsyncwire.NewStream(rw, maxFrame))
	if _, err := cl.SendSignatures(orig); err != nil {
		remoteStdin.Close()
		return fmt.Errorf("send signatures: %w", err)
	}
	if _, err := orig.Seek(0, io.SeekStart); err != nil {
		remoteStdin.Close()
		return fmt.Errorf("seek origin: %w", err)
	}

	const chunk = 32 * 1024
	bufSnap := make([]byte, chunk)
	bufOrig := make([]byte, chunk)
	var off int64
	for {
		nSnap, errSnap := snap.Read(bufSnap)
		nOrig, errOrig := orig.Read(bufOrig)
		n := nSnap
		if nOrig < n {
			n = nOrig
		}
		if n > 0 {
			i := 0
			for i < n {
				if bufSnap[i] != bufOrig[i] {
					start := i
					for i < n && bufSnap[i] != bufOrig[i] {
						i++
					}
					if err := cl.SendDelta(off+int64(start), bufSnap[start:i]); err != nil {
						remoteStdin.Close()
						return fmt.Errorf("send delta: %w", err)
					}
				} else {
					i++
				}
			}
			off += int64(n)
		}
		if errSnap == io.EOF || errOrig == io.EOF {
			break
		}
		if errSnap != nil {
			remoteStdin.Close()
			return fmt.Errorf("read snapshot: %w", errSnap)
		}
		if errOrig != nil {
			remoteStdin.Close()
			return fmt.Errorf("read origin: %w", errOrig)
		}
	}

	sum, err := r.sumFile(snapshotDevice, alg)
	if err != nil {
		remoteStdin.Close()
		return fmt.Errorf("compute digest: %w", err)
	}
	if err := cl.SendDigest(alg, sum); err != nil {
		remoteStdin.Close()
		return fmt.Errorf("send digest: %w", err)
	}
	if err := remoteStdin.Close(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to close remote stdin: %w", err)
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

// ExecuteRemoteCommand runs the remote command over SSH.
func (r *Runner) ExecuteRemoteCommand(ctx context.Context, cfg *config.Config, client *remote.SSHClient, destDevice, snapshotDevice, originDevice, alg string, logger *zap.Logger) (err error) {
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

	baseCmd := cfg.LVMSyncPath
	if cfg.DestType != "" && cfg.DestType != "auto" {
		baseCmd = fmt.Sprintf("%s --dest-type %s", baseCmd, cfg.DestType)
	}
	remoteCmd := fmt.Sprintf("%s --digest %s --verify %s %s", baseCmd, alg, cfg.VerifyLevel, destDevice)
	logger.Info("Starting remote command", zap.String("command", remoteCmd))

	if err = session.Start(remoteCmd); err != nil {
		return fmt.Errorf("failed to start remote command: %w", err)
	}

	if err = r.streamToRemote(ctx, cfg, remoteStdin, snapshotDevice, originDevice, alg, logger); err != nil {
		return err
	}

	return WaitForRemoteCompletion(session, stdoutErrCh, stderrErrCh)
}

// RunRemoteDump streams snapshot data to a remote host over SSH.
func (r *Runner) RunRemoteDump(ctx context.Context, cfg *config.Config, snapshotDevice, originDevice, dest string, logger *zap.Logger) (err error) {
	if cfg.DryRun {
		return r.ExecuteDump(ctx, cfg, snapshotDevice, originDevice, io.Discard, logger)
	}
	parts := strings.SplitN(dest, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid destination %q: expected host:device", dest)
	}
	destHost, destDevice := parts[0], parts[1]
	client, cancel, err := r.SetupSSHClient(ctx, cfg, destHost, logger)
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

	alg := strings.ToLower(cfg.ChecksumAlgorithm)
	if alg == "" || alg == "auto" {
		alg = digestpkg.Select()
	}
	logger.Info("digest_selected",
		zap.String("digest_alg", alg),
		zap.Bool("avx2", cpufeatures.HasAVX2()),
		zap.Bool("avx512", cpufeatures.HasAVX512()),
		zap.Bool("neon", cpufeatures.HasNEON()),
	)
	validationCtx, cancel := context.WithTimeout(ctx, cfg.SSHTimeout)
	defer cancel()
	return r.ExecuteRemoteCommand(validationCtx, cfg, client, destDevice, snapshotDevice, originDevice, alg, logger)
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
		tr, err := transport.Get(
			name,
			transport.Config{
				Logger:        logger,
				SSHUser:       cfg.SSHUser,
				SSHPassword:   cfg.SSHPassword,
				SSHKnownHosts: cfg.KnownHosts,
				SSHHostKey:    cfg.SSHHostKey,
				HostKeyPath:   cfg.SSHHostKeyPath,
				AllowInsecure: cfg.AllowInsecure,
			},
		)
		if err != nil {
			logger.Warn("unsupported transport", zap.String("transport", name))
			continue
		}
		logger.Info("selected transport", zap.String("transport", name))
		return tr, nil
	}
	return nil, fmt.Errorf("unsupported transport: %s", cfg.Transport)
}
