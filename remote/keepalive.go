// remote/keepalive.go
package remote

import (
	"time"

	"go.uber.org/zap"
)

func (c *SSHClient) startKeepAlive(host string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := c.sendKeepAlive(host); err != nil {
			break
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
