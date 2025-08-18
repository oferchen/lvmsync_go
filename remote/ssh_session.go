// remote/ssh_session.go
package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

type SSHSession struct {
	Client  *SSHClient
	Session *ssh.Session
}

type SSHMultiplexer struct {
	mu       sync.Mutex
	sessions map[string]*SSHSession
}

var multiplexer = &SSHMultiplexer{
	sessions: make(map[string]*SSHSession),
}

func GetMultiplexedSession(client *SSHClient, host string) (*SSHSession, error) {
	multiplexer.mu.Lock()
	defer multiplexer.mu.Unlock()

	if session, exists := multiplexer.sessions[host]; exists {
		return session, nil
	}

	session, err := NewSSHSession(client)
	if err != nil {
		return nil, err
	}

	multiplexer.sessions[host] = session
	return session, nil
}

func NewSSHSession(client *SSHClient) (*SSHSession, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}
	return &SSHSession{Client: client, Session: session}, nil
}

func (s *SSHSession) Start(command string, stdin io.Reader, stdout, stderr io.Writer) error {
	s.Session.Stdin = stdin
	s.Session.Stdout = stdout
	s.Session.Stderr = stderr
	return s.Session.Start(command)
}

func (s *SSHSession) Wait() error {
	return s.Session.Wait()
}

func (s *SSHSession) Close() {
	if err := s.Session.Close(); err != nil && !errors.Is(err, io.EOF) {
		s.Client.Logger.Warn("session close error", zap.Error(err))
	}
}

// RunSSHCommand executes a command on a remote host over SSH and logs using logger.
// stdout and stderr direct command output; nil values are replaced with io.Discard.
// logger must be non-nil; pass zap.NewNop() to disable logging.
func RunSSHCommand(ctx context.Context, logger *zap.Logger, host, user, keyPath, hostKeyPath string, port int, command string, timeout time.Duration, stdout, stderr io.Writer) error {
	publicKey, err := readHostPublicKey(hostKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load host public key: %w", err)
	}
	signer, err := readPrivateKey(keyPath)
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}
	client, err := dialSSH(ctx, fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(publicKey),
		Timeout:         timeout,
	}, timeout)
	if err != nil {
		return fmt.Errorf("failed to establish SSH connection: %w", err)
	}
	sshClient := &SSHClient{Client: client, Logger: logger}
	defer func() {
		if closeErr := sshClient.Close(); closeErr != nil {
			logger.Warn("client close error", zap.Error(closeErr))
		}
	}()

	session, err := NewSSHSession(sshClient)
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	errCh := make(chan error, 1)
	if err := session.Start(command, nil, stdout, stderr); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}
	go func() { errCh <- session.Wait() }()

	select {
	case <-ctx.Done():
		if sigErr := session.Session.Signal(ssh.SIGKILL); sigErr != nil && !errors.Is(sigErr, io.EOF) {
			logger.Warn("session signal error", zap.Error(sigErr))
		}
		<-errCh
		return ctx.Err()
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("SSH command failed: %w", err)
		}
	}

	logger.Info("SSH command completed", zap.String("host", host), zap.String("command", command))
	return nil
}

func readPrivateKey(keyPath string) (ssh.Signer, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read SSH private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse SSH private key: %w", err)
	}
	return signer, nil
}

// readHostPublicKey loads the allowed host public key from the given path and parses it.
func readHostPublicKey(keyPath string) (ssh.PublicKey, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read SSH host public key: %w", err)
	}
	publicKey, err := ssh.ParsePublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse SSH host public key: %w", err)
	}
	return publicKey, nil
}
