// remote/keepalive.go
package remote

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (c *SSHClient) startKeepAlive(ctx context.Context, host string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.sendKeepAlive(host); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *SSHClient) sendKeepAlive(host string) error {
	_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
	if err != nil {
		c.Logger.Warn("SSH keepalive failed", zap.String("host", host), zap.Error(err))
		return err
	}
	return nil
}
