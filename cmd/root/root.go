package root

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"lvmsync_go/app"
	applycmd "lvmsync_go/cmd/apply"
	dumpcmd "lvmsync_go/cmd/dump"
	manifestcmd "lvmsync_go/cmd/manifest"
	verifycmd "lvmsync_go/cmd/verify"
	"lvmsync_go/config"
	"lvmsync_go/device"
	clientpkg "lvmsync_go/internal/client"
	"lvmsync_go/internal/privilege"
)

// function variables for testing
var (
	startGRPCServer   = app.StartGRPCServer
	clientHandshake   = app.ClientHandshake
	setupSignalHandle = app.SetupSignalHandling
	prepareSnapshotFn = app.PrepareSnapshot
	executeClientFn   = clientpkg.ExecuteClient
	selectTransport   = dumpcmd.SelectTransport
	runDump           = func(ctx context.Context, cfg *config.Config, snapshot, dest string, logger *zap.Logger) error {
		_, err := dumpcmd.Run(ctx, cfg, snapshot, dest, logger)
		return err
	}
)

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
	flagSets := config.NewFlagSets(defaults)
	fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	cfg, args, err := config.LoadConfig(flagSets, defaults, fs, os.Args[1:])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configuration error: %w", err)
	}
	esc := privilege.New()
	if err = esc.Ensure(); err != nil {
		return nil, nil, nil, fmt.Errorf("privilege check failed: %w", err)
	}
	if err = cfg.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("configuration validation error: %w", err)
	}
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("logger initialization error: %w", err)
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
func SetupGRPC(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(), func(), chan error, error) {
	cleanupSrv, srvErrCh, err := startGRPCServer(ctx, cfg, logger)
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
	cleanupClient, hbErrCh, err := clientHandshake(ctx, cfg, logger)
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
func PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return prepareSnapshotFn(ctx, cfg, originalVolume, logger)
}

// ExecuteClient runs the client transfer logic.
func ExecuteClient(ctx context.Context, cfg *config.Config, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error, logger *zap.Logger) error {
	return executeClientFn(ctx, func(ctx context.Context, snapshot, dest string) error {
		return runDump(ctx, cfg, snapshot, dest, logger)
	}, snapshotPath, destPath, sigErrCh, monitorErrCh)
}

// Run orchestrates the command execution.
func Run(cfg *config.Config, args []string, logger *zap.Logger) error {
	if len(args) > 0 {
		switch args[0] {
		case "manifest":
			return manifestcmd.Run(cfg, args[1:], logger)
		case "verify":
			return verifycmd.Run(args[1:], logger)
		}
	}

	if cfg.ApplyMode != "" {
		if err := applycmd.Run(cfg, cfg.ApplyMode, args, logger); err != nil {
			return fmt.Errorf("apply operation failed: %w", err)
		}
		return nil
	}

	if _, err := selectTransport(cfg, logger); err != nil {
		return fmt.Errorf("select transport: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.GRPCSetupTimeout)
	cleanupSrv, cleanupClient, hbErrCh, err := SetupGRPC(ctx, cfg, logger)
	if err != nil {
		cancel()
		return err
	}
	defer func() {
		cancel()
		cleanupSrv()
	}()
	defer cleanupClient()

	var snapshotPath string
	signals, sigErrCh := setupSignalHandle(ctx, cfg, &snapshotPath, logger)
	defer signal.Stop(signals)

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
		return fmt.Errorf("invalid arguments")
	}
	originalVolume := args[0]
	destPath := ""
	if !cfg.StdoutMode {
		destPath = args[1]
	}

	snapshotPath = originalVolume
	var (
		monitorErrCh chan error
		cleanup      = func() {}
	)

	if cfg.SourceType == "" || cfg.SourceType == "auto" {
		dev, err := device.Detect(ctx, originalVolume, cfg.Offline, cfg.SourceType, cfg.FSFreezeCommand, cfg.FSThawCommand, cfg.LVMEscalation, cfg.FreezeTimeout, cfg.ThawTimeout, logger)
		if err != nil {
			return err
		}
		switch dev.(type) {
		case *device.LVMDevice:
			cfg.SourceType = "lvm"
		case *device.RawDevice:
			cfg.SourceType = "raw"
		case *device.FileDevice:
			cfg.SourceType = "file"
		}
		dev.Close()
	}
	switch cfg.SourceType {
	case "lvm":
		snapshotPath, monitorErrCh, cleanup, err = PrepareSnapshot(ctx, cfg, originalVolume, logger)
		if err != nil {
			return err
		}
	case "raw":
		if !cfg.SkipSnapshotCreation {
			return fmt.Errorf("raw sources require --skip_snapshot_creation and either --offline or --fs-freeze-command/--fs-thaw-command")
		}
	case "file":
	default:
		return fmt.Errorf("unknown source type %q", cfg.SourceType)
	}
	defer cleanup()

	return ExecuteClient(ctx, cfg, snapshotPath, destPath, sigErrCh, monitorErrCh, logger)
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
