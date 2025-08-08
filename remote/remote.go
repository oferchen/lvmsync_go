// remote/remote.go
package remote

import (
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

// NewSSHClient establishes an SSH connection to the given host using either a
// private key or the local SSH agent for authentication. The connection is
// configured with a keep-alive mechanism and host key verification based on the
// provided known_hosts file.
func NewSSHClient(host, user, keyPath string, port int, knownHostsPath string, verify bool, timeout, keepAliveInterval time.Duration, retries int) (*ssh.Client, error) {
	authMethods, err := selectAuthMethods(keyPath)
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
	client, err := dialWithRetry(addr, config, host, port, retries)
	if err != nil {
		return nil, err
	}

	go startKeepAlive(client, host, keepAliveInterval)

	return client, nil
}

//revive:disable-next-line:cognitive-complexity
func selectAuthMethods(keyPath string) ([]ssh.AuthMethod, error) {
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
			conn, err := net.Dial("unix", sshAgentSock)
			if err == nil {
				agentClient := agent.NewClient(conn)
				authMethods = append(authMethods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
					defer func() {
						if cerr := conn.Close(); cerr != nil {
							Logger.Warn("ssh agent connection close error", zap.Error(cerr))
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

func dialWithRetry(addr string, config *ssh.ClientConfig, host string, port, retries int) (*ssh.Client, error) {
	var client *ssh.Client
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		client, err = ssh.Dial("tcp", addr, config)
		if err == nil {
			return client, nil
		}
		Logger.Warn("SSH dial failed",
			zap.String("host", host),
			zap.Int("port", port),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
		if attempt < retries {
			backoff := time.Duration(1<<attempt) * time.Second
			time.Sleep(backoff)
		}
	}
	Logger.Error("Unable to establish SSH connection", zap.String("host", host), zap.Int("port", port), zap.Error(err))
	return nil, fmt.Errorf("failed to dial SSH after %d attempts: %w", retries+1, err)
}

// ValidateRemoteCommand ensures that the provided command exists and is
// executable on the remote host by attempting to run it with a --version flag.
// It returns an error if the command is missing or cannot be executed.
func ValidateRemoteCommand(client *ssh.Client, remoteCmd string) error {
	tokens := strings.Fields(remoteCmd)
	if len(tokens) == 0 {
		return fmt.Errorf("remote command is empty")
	}
	cmd := tokens[0]
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session for validation: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil && !errors.Is(err, io.EOF) {
			Logger.Warn("session close error", zap.Error(err))
		}
	}()
	session.Stdout = io.Discard
	session.Stderr = io.Discard
	if err := session.Run(fmt.Sprintf("%s --version", cmd)); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			status := exitErr.ExitStatus()
			if status == 126 || status == 127 {
				return fmt.Errorf("remote command %s not found or not executable: %w", cmd, err)
			}
		}
		return fmt.Errorf("failed to run remote command %s: %w", cmd, err)
	}
	return nil
}

// RunRemoteScript executes the provided shell script on the remote host using
// the given SSH client.
func RunRemoteScript(client *ssh.Client, script string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session for script: %w", err)
	}
	defer func() {
		if err := session.Close(); err != nil && !errors.Is(err, io.EOF) {
			Logger.Warn("session close error", zap.Error(err))
		}
	}()
	Logger.Info("Running remote script", zap.String("script", script))
	return session.Run(script)
}

// Logger is the package-wide logger used for all remote SSH operations. By
// default it discards all logs.
var Logger = zap.NewNop()

// SetLogger sets the package-wide logger. If a nil logger is provided, logging
// is disabled by using a no-op logger.
func SetLogger(logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}
	Logger = logger
}
