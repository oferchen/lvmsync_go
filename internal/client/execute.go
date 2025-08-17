package client

import (
	"context"

	"go.uber.org/zap"
)

// ExecuteClient runs the client transfer logic and handles signal and monitor errors.
func ExecuteClient(ctx context.Context, runClient func(context.Context, string, string) error, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error, logger *zap.Logger) error {
	clientErrCh := make(chan error, 1)
	go func() {
		clientErrCh <- runClient(ctx, snapshotPath, destPath)
	}()

	select {
	case err := <-clientErrCh:
		if err != nil {
			logger.Error("copy operation failed", zap.Error(err))
			return err
		}
	case err := <-sigErrCh:
		if err != nil {
			logger.Error("signal error", zap.Error(err))
		}
		return err
	case <-ctx.Done():
		logger.Error("context canceled", zap.Error(ctx.Err()))
		return ctx.Err()
	}

	if monitorErrCh != nil {
		for {
			select {
			case err, ok := <-monitorErrCh:
				if !ok {
					return nil
				}
				if err != nil {
					logger.Error("snapshot monitor error", zap.Error(err))
					return err
				}
			case <-ctx.Done():
				logger.Error("context canceled", zap.Error(ctx.Err()))
				return ctx.Err()
			}
		}
	}

	return nil
}
