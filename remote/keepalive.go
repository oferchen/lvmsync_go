// remote/keepalive.go
package remote

import (
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func startKeepAlive(client *ssh.Client, host string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		err := sendKeepAlive(client, host)
		if err != nil {
			break
		}
	}
}

func sendKeepAlive(client *ssh.Client, host string) error {
	_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
	if err != nil {
		zap.L().Warn("SSH keepalive failed", zap.String("host", host), zap.Error(err))
		return err
	}
	return nil
}
