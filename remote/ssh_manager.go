// remote/ssh_manager.go
package remote

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHManager maintains SSH client connections for reuse and ensures
// consistent host key verification.
//
// It caches established connections keyed by host and port so that
// subsequent requests to the same destination reuse existing sessions.
// The manager is safe for concurrent use.
type SSHManager struct {
	mu        sync.Mutex
	clients   map[string]*ssh.Client
	sshConfig *ssh.ClientConfig
	timeout   time.Duration
}

// NewSSHManager initializes an SSHManager for the provided user. The keyPath
// specifies a private key to use for authentication; if empty, the SSH agent
// will be consulted. All host keys are verified against the provided
// knownHostsPath. The timeout applies to establishing new connections.
func NewSSHManager(user, keyPath string, timeout time.Duration, knownHostsPath string) (*SSHManager, error) {
	authMethods, err := getSSHAuthMethods(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authentication: %w", err)
	}

	hostKeyCallback, err := getHostKeyCallback(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to set up host key verification: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	return &SSHManager{
		clients:   make(map[string]*ssh.Client),
		sshConfig: sshConfig,
		timeout:   timeout,
	}, nil
}

// GetClient returns an SSH client connected to the specified host and port.
// If a connection already exists it is reused; otherwise a new connection is
// established.
func (s *SSHManager) GetClient(host string, port int) (*ssh.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", host, port)
	if client, exists := s.clients[addr]; exists {
		return client, nil
	}

	client, err := dialSSH(addr, s.sshConfig, s.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	s.clients[addr] = client
	return client, nil
}

// CloseAll terminates all managed SSH client connections and clears the cache.
// Any errors encountered while closing clients are logged and aggregated.
func (s *SSHManager) CloseAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs error
	for addr, client := range s.clients {
		if err := client.Close(); err != nil {
			Logger.Warn("client close error", zap.String("addr", addr), zap.Error(err))
			errs = multierr.Append(errs, fmt.Errorf("%s: %w", addr, err))
		}
		delete(s.clients, addr)
	}

	return errs
}

func getSSHAuthMethods(keyPath string) ([]ssh.AuthMethod, error) {
	authMethods := []ssh.AuthMethod{}

	if keyPath != "" {
		signer, err := loadPrivateKey(keyPath)
		if err != nil {
			return nil, err
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		agentAuth, err := sshAgentAuth()
		if err != nil {
			return nil, err
		}
		authMethods = append(authMethods, agentAuth)
	}

	return authMethods, nil
}

// getHostKeyCallback returns a HostKeyCallback that verifies remote hosts
// against the known hosts file at knownHostsPath.
func getHostKeyCallback(knownHostsPath string) (ssh.HostKeyCallback, error) {
	return knownhosts.New(knownHostsPath)
}

func dialSSH(addr string, sshConfig *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	return ssh.NewClient(clientConn, chans, reqs), nil
}

func loadPrivateKey(keyPath string) (ssh.Signer, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	return ssh.ParsePrivateKey(keyData)
}

func sshAgentAuth() (ssh.AuthMethod, error) {
	agentSock := os.Getenv("SSH_AUTH_SOCK")
	if agentSock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK is not set")
	}

	conn, err := net.Dial("unix", agentSock)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
	}

	agentClient := agent.NewClient(conn)
	defer func() {
		if err := conn.Close(); err != nil {
			Logger.Warn("ssh agent connection close error", zap.Error(err))
		}
	}()

	signers, err := agentClient.Signers()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve signers from SSH agent: %w", err)
	}

	return ssh.PublicKeys(signers...), nil
}
