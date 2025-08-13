package remote

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestDialSSHContextCancel ensures dialSSH respects context cancellation.
func TestDialSSHContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel()
	cfg := &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	_, err := dialSSH(ctx, "192.0.2.1:22", cfg, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

// TestDialSSHTimeout ensures dialSSH respects dial timeouts.
func TestDialSSHTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cfg := &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	_, err := dialSSH(ctx, "198.51.100.1:22", cfg, time.Nanosecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	var netErr net.Error
	if !(errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())) {
		t.Fatalf("expected timeout error, got %v", err)
	}
}
