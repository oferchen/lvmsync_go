// remote/remote.go
package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHClient wraps an ssh.Client and provides structured logging.
//
// The embedded *zap.Logger defaults to a no-op logger when nil to avoid
// nil-pointer dereferences while still allowing callers to inject their own
// logger for observability.
type SSHClient struct {
	*ssh.Client
	*zap.Logger
}

// NewSSHClient establishes an SSH connection to the given host using either a
// private key or the local SSH agent for authentication. The connection is
// configured with a keep-alive mechanism and host key verification based on the
// provided known_hosts file.
func NewSSHClient(
	ctx context.Context,
	host, user, keyPath string,
	port int,
	knownHostsPath string,
	verify bool,
	timeout, keepAliveInterval time.Duration,
	retries int,
	logger *zap.Logger,
) (*SSHClient, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	authMethods, err := selectAuthMethods(logger, keyPath)
	if err != nil {
		return nil, err
	}
	hostKeyCallback, err := setupHostKeyCallback(verify, knownHostsPath)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := dialWithRetry(logger, addr, config, host, port, retries)
	if err != nil {
		return nil, err
	}

	sshClient := &SSHClient{Client: client, Logger: logger}
	go sshClient.startKeepAlive(ctx, host, keepAliveInterval)

	return sshClient, nil
}

//revive:disable-next-line:cognitive-complexity
func selectAuthMethods(logger *zap.Logger, keyPath string) ([]ssh.AuthMethod, error) {
	var authMethods []ssh.AuthMethod
	if keyPath != "" {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read SSH key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		sshAgentSock := os.Getenv("SSH_AUTH_SOCK")
		if sshAgentSock != "" {
			conn, err := net.DialTimeout("unix", sshAgentSock, 5*time.Second)
			if err != nil {
				logger.Warn("ssh agent dial failed", zap.String("sock", sshAgentSock), zap.Error(err))
			} else {
				agentClient := agent.NewClient(conn)
				authMethods = append(authMethods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
					defer func() {
						if cerr := conn.Close(); cerr != nil {
							logger.Warn("ssh agent connection close error", zap.Error(cerr))
						}
					}()
					return agentClient.Signers()
				}))
			}
		}
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods configured")
	}
	return authMethods, nil
}

func setupHostKeyCallback(_ bool, knownHostsPath string) (ssh.HostKeyCallback, error) {
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create knownhosts callback: %w", err)
	}
	return hostKeyCallback, nil
}

func dialWithRetry(logger *zap.Logger, addr string, config *ssh.ClientConfig, host string, port, retries int) (*ssh.Client, error) {
	var client *ssh.Client
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		client, err = ssh.Dial("tcp", addr, config)
		if err == nil {
			return client, nil
		}
		logger.Warn("SSH dial failed",
			zap.String("host", host),
			zap.Int("port", port),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
		if attempt < retries {
			backoff := time.Duration(1<<attempt) * time.Second
			time.Sleep(backoff)
		}
	}
	logger.Error("Unable to establish SSH connection", zap.String("host", host), zap.Int("port", port), zap.Error(err))
	return nil, fmt.Errorf("failed to dial SSH after %d attempts: %w", retries+1, err)
}

// ValidateRemoteCommand ensures that the provided command exists and is
// executable on the remote host by attempting to run it with a --version flag.
// It returns an error if the command is missing or cannot be executed.
func (c *SSHClient) ValidateRemoteCommand(ctx context.Context, remoteCmd string) error {
	tokens := strings.Fields(remoteCmd)
	if len(tokens) == 0 {
		return fmt.Errorf("remote command is empty")
	}
	cmd := tokens[0]
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session for validation: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil && !errors.Is(err, io.EOF) {
			c.Logger.Warn("session close error", zap.Error(err))
		}
	}()
	session.Stdout = io.Discard
	session.Stderr = io.Discard
	errCh := make(chan error, 1)
	if err := session.Start(fmt.Sprintf("%s --version", cmd)); err != nil {
		return fmt.Errorf("failed to start remote command %s: %w", cmd, err)
	}
	go func() { errCh <- session.Wait() }()
	select {
	case <-ctx.Done():
		if sigErr := session.Signal(ssh.SIGKILL); sigErr != nil && !errors.Is(sigErr, io.EOF) {
			c.Logger.Warn("session signal error", zap.Error(sigErr))
		}
		return ctx.Err()
	case err := <-errCh:
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				status := exitErr.ExitStatus()
				if status == 126 || status == 127 {
					return fmt.Errorf("remote command %s not found or not executable: %w", cmd, err)
				}
			}
			return fmt.Errorf("failed to run remote command %s: %w", cmd, err)
		}
	}
	return nil
}

// RunRemoteScript executes the provided shell script on the remote host using
// the given SSH client.
func (c *SSHClient) RunRemoteScript(ctx context.Context, script string) error {
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session for script: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil && !errors.Is(err, io.EOF) {
			c.Logger.Warn("session close error", zap.Error(err))
		}
	}()
	c.Logger.Info("Running remote script", zap.String("script", script))
	errCh := make(chan error, 1)
	if err := session.Start(script); err != nil {
		return fmt.Errorf("failed to start remote script: %w", err)
	}
	go func() { errCh <- session.Wait() }()
	select {
	case <-ctx.Done():
		if sigErr := session.Signal(ssh.SIGKILL); sigErr != nil && !errors.Is(sigErr, io.EOF) {
			c.Logger.Warn("session signal error", zap.Error(sigErr))
		}
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
