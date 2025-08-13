// remote/ssh_manager.go
package remote

import (
	"context"
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

// parsePrivateKey is a variable to allow test stubbing.
var parsePrivateKey = ssh.ParsePrivateKey

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
	logger    *zap.Logger
}

// NewSSHManager initializes an SSHManager for the provided user. The keyPath
// specifies a private key to use for authentication; if empty, the SSH agent
// will be consulted. All host keys are verified against the provided
// knownHostsPath. The timeout applies to establishing new connections.
func NewSSHManager(user, keyPath string, timeout time.Duration, knownHostsPath string, logger *zap.Logger) (*SSHManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	authMethods, err := getSSHAuthMethods(ctx, keyPath, timeout, logger)
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
		logger:    logger,
	}, nil
}

// GetClient returns an SSH client connected to the specified host and port.
// If a connection already exists it is reused; otherwise a new connection is
// established.
func (s *SSHManager) GetClient(ctx context.Context, host string, port int) (*ssh.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", host, port)
	if client, exists := s.clients[addr]; exists {
		return client, nil
	}

	client, err := dialSSH(ctx, addr, s.sshConfig, s.timeout)
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
			s.logger.Warn("client close error", zap.String("addr", addr), zap.Error(err))
			errs = multierr.Append(errs, fmt.Errorf("%s: %w", addr, err))
		}
		delete(s.clients, addr)
	}

	return errs
}

func getSSHAuthMethods(ctx context.Context, keyPath string, timeout time.Duration, logger *zap.Logger) ([]ssh.AuthMethod, error) {
	authMethods := []ssh.AuthMethod{}

	if keyPath != "" {
		signer, err := loadPrivateKey(keyPath)
		if err != nil {
			return nil, err
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		agentAuth, err := sshAgentAuth(ctx, timeout, logger)
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

func dialSSH(ctx context.Context, addr string, sshConfig *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	dialer := net.Dialer{Timeout: timeout}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Deadline = deadline
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
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
	signer, err := parsePrivateKey(keyData)
	// Zero the key material to avoid leaving it in memory.
	for i := range keyData {
		keyData[i] = 0
	}
	return signer, err
}

func sshAgentAuth(ctx context.Context, timeout time.Duration, logger *zap.Logger) (ssh.AuthMethod, error) {
	agentSock := os.Getenv("SSH_AUTH_SOCK")
	if agentSock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK is not set")
	}

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "unix", agentSock)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
	}

	agentClient := agent.NewClient(conn)
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn("ssh agent connection close error", zap.Error(closeErr))
		}
	}()

	signers, err := agentClient.Signers()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve signers from SSH agent: %w", err)
	}

	return ssh.PublicKeys(signers...), nil
}
