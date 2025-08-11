package root

import (
	"fmt"
	"os/signal"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"lvmsync_go/app"
	applycmd "lvmsync_go/cmd/apply"
	dumpcmd "lvmsync_go/cmd/dump"
	servecmd "lvmsync_go/cmd/serve"
	"lvmsync_go/config"
	clientpkg "lvmsync_go/internal/client"
	"lvmsync_go/internal/privilege"
	"lvmsync_go/lvm"
)

// function variables for testing
var (
	startGRPCServer   = app.StartGRPCServer
	clientHandshake   = app.ClientHandshake
	setupSignalHandle = app.SetupSignalHandling
	prepareSnapshotFn = app.PrepareSnapshot
	executeClientFn   = clientpkg.ExecuteClient
	selectTransport   = dumpcmd.SelectTransport
	runDump           = dumpcmd.Run
	runServe          = servecmd.Run
)

// SyncLogger flushes buffered log entries and logs if syncing fails.
func SyncLogger(logger *zap.Logger) {
	if err := logger.Sync(); err != nil {
		logger.Error("Logger sync error", zap.Error(err))
	}
}

// Configure loads configuration, ensures privileges, validates, and sets up logging.
func Configure() (*config.Config, *zap.Logger, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("configuration error: %w", err)
	}
	esc := privilege.New()
	if err = esc.Ensure(); err != nil {
		return nil, nil, fmt.Errorf("privilege check failed: %w", err)
	}
	if err = cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("configuration validation error: %w", err)
	}
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, nil, fmt.Errorf("logger initialization error: %w", err)
	}
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
	return cfg, logger, nil
}

// SetupGRPC starts the server and performs client handshake returning cleanup functions.
func SetupGRPC(cfg *config.Config, logger *zap.Logger) (func(), func(), error) {
	cleanupSrv, err := startGRPCServer(cfg, logger)
	if err != nil {
		return nil, nil, err
	}
	cleanupClient, err := clientHandshake(cfg, logger)
	if err != nil {
		cleanupSrv()
		return nil, nil, err
	}
	return cleanupSrv, cleanupClient, nil
}

// PrepareSnapshot wraps snapshot preparation.
func PrepareSnapshot(cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return prepareSnapshotFn(cfg, originalVolume, logger)
}

// ExecuteClient runs the client transfer logic.
func ExecuteClient(cfg *config.Config, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error, logger *zap.Logger) error {
	return executeClientFn(func(snapshot, dest string) error {
		return runDump(cfg, snapshot, dest, logger)
	}, snapshotPath, destPath, sigErrCh, monitorErrCh)
}

// Run orchestrates the command execution.
func Run(cfg *config.Config, logger *zap.Logger) error {
	if cfg.Serve {
		return runServe(cfg, logger)
	}

	defer lvm.Cleanup()

	if err := selectTransport(cfg, logger); err != nil {
		return err
	}

	cleanupSrv, cleanupClient, err := SetupGRPC(cfg, logger)
	if err != nil {
		return err
	}
	defer cleanupSrv()
	defer cleanupClient()

	var snapshotPath string
	signals, sigErrCh := setupSignalHandle(cfg, &snapshotPath)
	defer signal.Stop(signals)

	if cfg.ApplyMode != "" {
		if err = applycmd.Run(cfg, cfg.ApplyMode, pflag.Args()); err != nil {
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
	snapshotPath, monitorErrCh, cleanup, err := PrepareSnapshot(cfg, originalVolume, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	return ExecuteClient(cfg, snapshotPath, destPath, sigErrCh, monitorErrCh, logger)
}

// Execute is a helper that configures and runs the root command.
func Execute() error {
	cfg, logger, err := Configure()
	if err != nil {
		return err
	}
	if err := Run(cfg, logger); err != nil {
		logger.Error("run failed", zap.Error(err))
		SyncLogger(logger)
		return err
	}
	SyncLogger(logger)
	return nil
}
