// remote/ssh_session.go
package remote

import (
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
	Client  *ssh.Client
	Session *ssh.Session
}

type SSHMultiplexer struct {
	mu       sync.Mutex
	sessions map[string]*SSHSession
}

var multiplexer = &SSHMultiplexer{
	sessions: make(map[string]*SSHSession),
}

func GetMultiplexedSession(client *ssh.Client, host string) (*SSHSession, error) {
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

func NewSSHSession(client *ssh.Client) (*SSHSession, error) {
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
		Logger.Warn("session close error", zap.Error(err))
	}
}

func RunSSHCommand(host, user, keyPath, hostKeyPath string, port int, command string, timeout time.Duration) error {
	publicKey, err := readHostPublicKey(hostKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load host public key: %w", err)
	}
	signer, err := readPrivateKey(keyPath)
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}
	client, err := dialSSH(fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(publicKey),
		Timeout:         timeout,
	}, timeout)
	if err != nil {
		return fmt.Errorf("failed to establish SSH connection: %w", err)
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			Logger.Warn("client close error", zap.Error(closeErr))
		}
	}()

	session, err := NewSSHSession(client)
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf io.Writer
	cmdErr := session.Start(command, nil, stdoutBuf, stderrBuf)
	if cmdErr != nil {
		return fmt.Errorf("failed to start command: %w", cmdErr)
	}

	err = session.Wait()
	if err != nil {
		return fmt.Errorf("SSH command failed: %w", err)
	}

	Logger.Info("SSH command completed", zap.String("host", host), zap.String("command", command))
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
