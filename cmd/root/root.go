package root

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"lvmsync_go/app"
	clientpkg "lvmsync_go/internal/client"
	"lvmsync_go/internal/config"
	"lvmsync_go/internal/logging"
	"lvmsync_go/internal/privilege"
	"lvmsync_go/transport"
)

// Runner manages external interactions for the root command.
type Runner struct {
	startGRPCServerFn   func(context.Context, *config.Config, *zap.Logger) (func(), <-chan error, error)
	clientHandshakeFn   func(context.Context, *config.Config, *zap.Logger) (func(), chan error, error)
	setupSignalHandleFn func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error)
	prepareSnapshotFn   func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error)
	executeClientFn     func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error
	runApplyFn          func(cfg *config.Config, applyFile string, args []string, logger *zap.Logger) error
	selectTransportFn   func(cfg *config.Config, logger *zap.Logger) (transport.Interface, error)
	runDumpFn           func(ctx context.Context, cfg *config.Config, snapshot, dest string, logger *zap.Logger) (string, error)
	runManifestFn       func(cfg *config.Config, args []string, logger *zap.Logger) error
	runVerifyFn         func(args []string, logger *zap.Logger) error
}

// NewRunner constructs a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{
		startGRPCServerFn:   app.StartGRPCServer,
		clientHandshakeFn:   app.ClientHandshake,
		setupSignalHandleFn: app.SetupSignalHandling,
		prepareSnapshotFn:   app.PrepareSnapshot,
		executeClientFn:     clientpkg.ExecuteClient,
		runApplyFn: func(cfg *config.Config, applyFile string, args []string, logger *zap.Logger) error {
			return fmt.Errorf("apply command not registered")
		},
		selectTransportFn: func(cfg *config.Config, logger *zap.Logger) (transport.Interface, error) {
			return nil, fmt.Errorf("transport not registered")
		},
		runDumpFn: func(ctx context.Context, cfg *config.Config, snapshot, dest string, logger *zap.Logger) (string, error) {
			return "", fmt.Errorf("dump command not registered")
		},
		runManifestFn: func(cfg *config.Config, args []string, logger *zap.Logger) error {
			return fmt.Errorf("manifest command not registered")
		},
		runVerifyFn: func(args []string, logger *zap.Logger) error {
			return fmt.Errorf("verify command not registered")
		},
	}
}

// NewRunnerWithDeps constructs a Runner overriding default dependencies.
func NewRunnerWithDeps(deps *Runner) *Runner {
	r := NewRunner()
	if deps == nil {
		return r
	}
	if deps.startGRPCServerFn != nil {
		r.startGRPCServerFn = deps.startGRPCServerFn
	}
	if deps.clientHandshakeFn != nil {
		r.clientHandshakeFn = deps.clientHandshakeFn
	}
	if deps.setupSignalHandleFn != nil {
		r.setupSignalHandleFn = deps.setupSignalHandleFn
	}
	if deps.prepareSnapshotFn != nil {
		r.prepareSnapshotFn = deps.prepareSnapshotFn
	}
	if deps.executeClientFn != nil {
		r.executeClientFn = deps.executeClientFn
	}
	if deps.runApplyFn != nil {
		r.runApplyFn = deps.runApplyFn
	}
	if deps.selectTransportFn != nil {
		r.selectTransportFn = deps.selectTransportFn
	}
	if deps.runDumpFn != nil {
		r.runDumpFn = deps.runDumpFn
	}
	if deps.runManifestFn != nil {
		r.runManifestFn = deps.runManifestFn
	}
	if deps.runVerifyFn != nil {
		r.runVerifyFn = deps.runVerifyFn
	}
	return r
}

var defaultRunner = NewRunner()

// RegisterApply sets the apply handler used by the default Runner.
func RegisterApply(fn func(*config.Config, string, []string, *zap.Logger) error) {
	defaultRunner.runApplyFn = fn
}

// RegisterDump sets the dump handler used by the default Runner.
func RegisterDump(fn func(context.Context, *config.Config, string, string, *zap.Logger) (string, error)) {
	defaultRunner.runDumpFn = fn
}

// RegisterSelectTransport sets the transport selector used by the default Runner.
func RegisterSelectTransport(fn func(*config.Config, *zap.Logger) (transport.Interface, error)) {
	defaultRunner.selectTransportFn = fn
}

// RegisterManifest sets the manifest handler used by the default Runner.
func RegisterManifest(fn func(*config.Config, []string, *zap.Logger) error) {
	defaultRunner.runManifestFn = fn
}

// RegisterVerify sets the verify handler used by the default Runner.
func RegisterVerify(fn func([]string, *zap.Logger) error) {
	defaultRunner.runVerifyFn = fn
}

// SyncLogger flushes buffered log entries and logs if syncing fails.
func SyncLogger(logger *zap.Logger) {
	if err := logger.Sync(); err != nil {
		logger.Error("Logger sync error", zap.Error(err))
	}
}

// Configure loads configuration, ensures privileges, validates, and sets up logging.
func Configure() (*config.Config, []string, *zap.Logger, error) {
	defaults, err := config.DefaultConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configuration error: %w", err)
	}
	builder := config.NewBuilder(defaults)
	fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	cfg, args, warns, err := builder.Build(fs, os.Args[1:])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configuration error: %w", err)
	}
	esc := privilege.New()
	if err = esc.Ensure(context.Background()); err != nil {
		return nil, nil, nil, fmt.Errorf("privilege check failed: %w", err)
	}
	if err = cfg.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("configuration validation error: %w", err)
	}
	logger, err := logging.NewLogger(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("logger initialization error: %w", err)
	}
	for _, w := range warns {
		logger.Warn(w)
	}
	logger.Info("Effective configuration",
		zap.String("block_size", cfg.HumanBlockSize()),
		zap.Uint64("block_size_bytes", cfg.BlockSizeBytes()),
		zap.Int("parallel", cfg.Parallel),
		zap.String("transport", cfg.Transport),
		zap.Int("concurrency", cfg.Concurrency),
		zap.Int("tcp_port", cfg.TCPPort),
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
	return cfg, args, logger, nil
}

// SetupGRPC starts the server and performs client handshake returning cleanup functions and heartbeat error channel.
func (r *Runner) SetupGRPC(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(), func(), chan error, error) {
	cleanupSrv, srvErrCh, err := r.startGRPCServerFn(ctx, cfg, logger)
	if err != nil {
		return nil, nil, nil, err
	}
	select {
	case err, ok := <-srvErrCh:
		if ok && err != nil {
			cleanupSrv()
			return nil, nil, nil, fmt.Errorf("gRPC serve: %w", err)
		}
	default:
	}
	cleanupClient, hbErrCh, err := r.clientHandshakeFn(ctx, cfg, logger)
	if err != nil {
		cleanupSrv()
		<-srvErrCh
		return nil, nil, nil, err
	}
	wrappedSrvCleanup := func() {
		cleanupSrv()
		<-srvErrCh
	}
	return wrappedSrvCleanup, cleanupClient, hbErrCh, nil
}

// PrepareSnapshot wraps snapshot preparation.
func (r *Runner) PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return r.prepareSnapshotFn(ctx, cfg, originalVolume, logger)
}

// ExecuteClient runs the client transfer logic.
func (r *Runner) ExecuteClient(ctx context.Context, cfg *config.Config, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error, logger *zap.Logger) error {
	return r.executeClientFn(ctx, func(ctx context.Context, snapshot, dest string) error {
		_, err := r.runDumpFn(ctx, cfg, snapshot, dest, logger)
		return err
	}, snapshotPath, destPath, sigErrCh, monitorErrCh, logger)
}

// SetupGRPC wraps the default runner's SetupGRPC.
func SetupGRPC(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(), func(), chan error, error) {
	return defaultRunner.SetupGRPC(ctx, cfg, logger)
}

// PrepareSnapshot wraps the default runner's PrepareSnapshot.
func PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return defaultRunner.PrepareSnapshot(ctx, cfg, originalVolume, logger)
}

// ExecuteClient wraps the default runner's ExecuteClient.
func ExecuteClient(ctx context.Context, cfg *config.Config, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error, logger *zap.Logger) error {
	return defaultRunner.ExecuteClient(ctx, cfg, snapshotPath, destPath, sigErrCh, monitorErrCh, logger)
}

// Run orchestrates the command execution.
func (r *Runner) dispatchSubcommand(cfg *config.Config, args []string, logger *zap.Logger) (bool, error) {
	if len(args) > 0 {
		switch args[0] {
		case "manifest":
			return true, r.runManifestFn(cfg, args[1:], logger)
		case "verify":
			return true, r.runVerifyFn(args[1:], logger)
		}
	}
	if cfg.ApplyMode != "" {
		if err := r.runApplyFn(cfg, cfg.ApplyMode, args, logger); err != nil {
			return true, fmt.Errorf("apply operation failed: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func (r *Runner) prepareClient(cfg *config.Config, args []string, logger *zap.Logger) (context.Context, func(), string, string, chan error, error) {
	if _, err := r.selectTransportFn(cfg, logger); err != nil {
		return nil, nil, "", "", nil, fmt.Errorf("select transport: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.GRPCSetupTimeout)
	cleanupSrv, cleanupClient, hbErrCh, err := r.SetupGRPC(ctx, cfg, logger)
	if err != nil {
		cancel()
		return nil, nil, "", "", nil, err
	}

	var snapshotPath string
	signals, sigErrCh := r.setupSignalHandleFn(ctx, cfg, &snapshotPath, logger)

	if hbErrCh != nil {
		go func() {
			if err := <-hbErrCh; err != nil {
				logger.Error("heartbeat error", zap.Error(err))
				sigErrCh <- err
			}
		}()
	}

	if (cfg.StdoutMode && len(args) < 1) || (!cfg.StdoutMode && len(args) < 2) {
		pflag.Usage()
		cancel()
		cleanupSrv()
		cleanupClient()
		signal.Stop(signals)
		return nil, nil, "", "", nil, fmt.Errorf("invalid arguments")
	}
	originalVolume := args[0]
	destPath := ""
	if !cfg.StdoutMode {
		destPath = args[1]
	}
	snapshotPath = originalVolume

	cleanup := func() {
		signal.Stop(signals)
		cancel()
		cleanupSrv()
		cleanupClient()
	}
	return ctx, cleanup, snapshotPath, destPath, sigErrCh, nil
}

func (r *Runner) executeSync(ctx context.Context, cfg *config.Config, snapshotPath, destPath string, sigErrCh chan error, logger *zap.Logger) error {
	return r.ExecuteClient(ctx, cfg, snapshotPath, destPath, sigErrCh, nil, logger)
}

func (r *Runner) Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	handled, err := r.dispatchSubcommand(cfg, args, logger)
	if handled || err != nil {
		return err
	}

	ctx, cleanup, snapshotPath, destPath, sigErrCh, err := r.prepareClient(cfg, args, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	return r.executeSync(ctx, cfg, snapshotPath, destPath, sigErrCh, logger)
}

// Run invokes the default runner's Run method.
func Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	return defaultRunner.Run(cfg, args, logger)
}

// Execute is a helper that configures and runs the root command.
func Execute() error {
	cfg, args, logger, err := Configure()
	if err != nil {
		return err
	}
	if err := Run(cfg, args, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		SyncLogger(logger)
		return err
	}
	SyncLogger(logger)
	return nil
}
