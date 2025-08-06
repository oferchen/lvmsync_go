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
				authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
			}
		}
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods configured")
	}
	return authMethods, nil
}

func setupHostKeyCallback(verify bool, knownHostsPath string) (ssh.HostKeyCallback, error) {
	if verify {
		hostKeyCallback, err := knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create knownhosts callback: %w", err)
		}
		return hostKeyCallback, nil
	}
	return ssh.InsecureIgnoreHostKey(), nil
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

var Logger = zap.NewNop()

func SetLogger(logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}
	Logger = logger
}
