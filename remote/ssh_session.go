// remote/ssh_session.go
package remote

import (
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
	s.Session.Close()
}

func RunSSHCommand(host, user, keyPath string, port int, command string, timeout time.Duration) error {
	client, err := dialSSH(fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(loadPrivateKeyMust(keyPath))},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}, timeout)

	if err != nil {
		return fmt.Errorf("failed to establish SSH connection: %w", err)
	}
	defer client.Close()

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

	zap.L().Info("SSH command completed", zap.String("host", host), zap.String("command", command))
	return nil
}

func loadPrivateKeyMust(keyPath string) ssh.Signer {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		zap.L().Fatal("Failed to read SSH private key", zap.String("keyPath", keyPath), zap.Error(err))
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		zap.L().Fatal("Failed to parse SSH private key", zap.String("keyPath", keyPath), zap.Error(err))
	}
	return signer
}
