// remote/remote.go
package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// remoteCmdRe matches allowable command names for remote execution.
var remoteCmdRe = regexp.MustCompile("^[a-zA-Z0-9._-]+$")

// ValidRemoteCommand reports whether cmd is a valid remote command name.
// It returns true if cmd matches remoteCmdRe.
func ValidRemoteCommand(cmd string) bool {
	return remoteCmdRe.MatchString(cmd)
}

// SSHClient wraps an ssh.Client and provides structured logging.
//
// The embedded *zap.Logger must be non-nil; callers can pass
// zap.NewNop() to disable logging.
type SSHClient struct {
	*ssh.Client
	*zap.Logger
}

// NewSSHClient establishes an SSH connection to the given host using either a
// private key or the local SSH agent for authentication. The connection is
// configured with a keep-alive mechanism and host key verification based on the
// provided known_hosts file. The logger must not be nil; use zap.NewNop() when
// logging is undesired.
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

	authMethods, err := selectAuthMethods(ctx, logger, keyPath, timeout)
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
	client, err := dialWithRetry(ctx, logger, addr, config, host, port, retries)
	if err != nil {
		return nil, err
	}

	sshClient := &SSHClient{Client: client, Logger: logger}
	go sshClient.startKeepAlive(ctx, host, keepAliveInterval)

	return sshClient, nil
}

func keyFileAuth(keyPath string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key file: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %w", err)
	}
	return ssh.PublicKeys(signer), nil
}

func agentAuth(ctx context.Context, logger *zap.Logger, timeout time.Duration) (ssh.AuthMethod, error) {
	sshAgentSock := os.Getenv("SSH_AUTH_SOCK")
	if sshAgentSock == "" {
		return nil, nil
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", sshAgentSock)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		logger.Warn("ssh agent dial failed", zap.String("socket_path", sshAgentSock), zap.Error(err))
		return nil, nil
	}
	agentClient := agent.NewClient(conn)
	return ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		defer func() {
			if cerr := conn.Close(); cerr != nil {
				logger.Warn("ssh agent connection close error", zap.Error(cerr))
			}
		}()
		return agentClient.Signers()
	}), nil
}

func aggregateAuthMethods(methods ...ssh.AuthMethod) ([]ssh.AuthMethod, error) {
	var result []ssh.AuthMethod
	for _, m := range methods {
		if m != nil {
			result = append(result, m)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods configured")
	}
	return result, nil
}

func selectAuthMethods(ctx context.Context, logger *zap.Logger, keyPath string, timeout time.Duration) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if keyPath != "" {
		keyMethod, err := keyFileAuth(keyPath)
		if err != nil {
			return nil, err
		}
		methods = append(methods, keyMethod)
	}
	agentMethod, err := agentAuth(ctx, logger, timeout)
	if err != nil {
		return nil, err
	}
	methods = append(methods, agentMethod)
	return aggregateAuthMethods(methods...)
}

func setupHostKeyCallback(verify bool, knownHostsPath string) (ssh.HostKeyCallback, error) {
	if !verify {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create knownhosts callback: %w", err)
	}
	return hostKeyCallback, nil
}

func dialWithRetry(ctx context.Context, logger *zap.Logger, addr string, config *ssh.ClientConfig, host string, port, retries int) (*ssh.Client, error) {
	var client *ssh.Client
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
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
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	logger.Error("Unable to establish SSH connection", zap.String("host", host), zap.Int("port", port), zap.Error(err))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, fmt.Errorf("failed to dial SSH after %d attempts: %w", retries+1, err)
}

// ValidateRemoteCommand ensures that the provided command exists and is
// executable on the remote host by attempting to run it with a --version flag.
// It returns an error if the command is missing or cannot be executed.
func (c *SSHClient) ValidateRemoteCommand(ctx context.Context, remoteCmd string) error {
	if strings.ContainsAny(remoteCmd, "&|;<>$`\"'!\n\r*") {
		return fmt.Errorf("remote command contains shell metacharacters")
	}
	tokens := strings.Fields(remoteCmd)
	if len(tokens) == 0 {
		return fmt.Errorf("remote command is empty")
	}
	cmd := filepath.Base(tokens[0])
	if !ValidRemoteCommand(cmd) {
		return fmt.Errorf("remote command %s contains invalid characters", cmd)
	}
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
