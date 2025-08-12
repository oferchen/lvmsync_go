package client

import (
	"context"
	"fmt"
)

// ExecuteClient runs the client transfer logic and handles signal and monitor errors.
func ExecuteClient(ctx context.Context, runClient func(context.Context, string, string) error, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error) error {
	clientErrCh := make(chan error, 1)
	go func() {
		clientErrCh <- runClient(ctx, snapshotPath, destPath)
	}()

	select {
	case err := <-clientErrCh:
		if err != nil {
			return fmt.Errorf("copy operation failed: %w", err)
		}
	case err := <-sigErrCh:
		return err
	case <-ctx.Done():
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
					return fmt.Errorf("snapshot monitor error: %w", err)
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return nil
}
