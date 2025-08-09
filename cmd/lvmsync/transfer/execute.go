package transfer

import "fmt"

// Execute runs the client transfer logic and handles signal and monitor errors.
func Execute(runClient func(string, string) error, snapshotPath, destPath string, sigErrCh, monitorErrCh chan error) error {
	clientErrCh := make(chan error, 1)
	go func() {
		clientErrCh <- runClient(snapshotPath, destPath)
	}()

	select {
	case err := <-clientErrCh:
		if err != nil {
			return fmt.Errorf("copy operation failed: %w", err)
		}
	case err := <-sigErrCh:
		return err
	}

	if monitorErrCh != nil {
		select {
		case err := <-monitorErrCh:
			if err != nil {
				return fmt.Errorf("snapshot monitor error: %w", err)
			}
		default:
		}
	}

	return nil
}
