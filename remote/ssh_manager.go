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

// SSHManager maintains SSH client connections for reuse and ensures
// consistent host key verification.
//
// It caches established connections keyed by host and port so that
// subsequent requests to the same destination reuse existing sessions.
// The manager is safe for concurrent use.
type sshClientEntry struct {
	client *ssh.Client
	done   chan error
}

type SSHManager struct {
	mu              sync.Mutex
	clients         map[string]*sshClientEntry
	sshConfig       *ssh.ClientConfig
	timeout         time.Duration
	logger          *zap.Logger
	parsePrivateKey func([]byte) (ssh.Signer, error)
}

// SSHManagerOption customizes the SSHManager configuration.
type SSHManagerOption func(*SSHManager)

// WithPrivateKeyParser sets a custom private key parser used to load
// authentication keys.
func WithPrivateKeyParser(p func([]byte) (ssh.Signer, error)) SSHManagerOption {
	return func(m *SSHManager) {
		m.parsePrivateKey = p
	}
}

// NewSSHManager initializes an SSHManager for the provided user. The keyPath
// specifies a private key to use for authentication; if empty, the SSH agent
// will be consulted. All host keys are verified against the provided
// knownHostsPath. The timeout applies to establishing new connections.
//
// The provided ctx controls cancellation for initialization steps such as
// retrieving authentication methods. It should include a deadline.
func NewSSHManager(ctx context.Context, user, keyPath string, timeout time.Duration, knownHostsPath string, logger *zap.Logger, opts ...SSHManagerOption) (*SSHManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	mgr := &SSHManager{
		clients:         make(map[string]*sshClientEntry),
		timeout:         timeout,
		logger:          logger,
		parsePrivateKey: ssh.ParsePrivateKey,
	}

	for _, opt := range opts {
		opt(mgr)
	}

	authMethods, err := getSSHAuthMethods(ctx, keyPath, timeout, logger, mgr.parsePrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authentication: %w", err)
	}

	hostKeyCallback, err := getHostKeyCallback(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to set up host key verification: %w", err)
	}

	mgr.sshConfig = &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	return mgr, nil
}

// GetClient returns an SSH client connected to the specified host and port.
// If a connection already exists it is reused; otherwise a new connection is
// established.
func (s *SSHManager) GetClient(ctx context.Context, host string, port int) (*ssh.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", host, port)
	if entry, exists := s.clients[addr]; exists {
		select {
		case err := <-entry.done:
			s.logger.Debug("ssh connection closed", zap.String("addr", addr), zap.Error(err))
			entry.client.Close() //nolint:errcheck
			delete(s.clients, addr)
		default:
			return entry.client, nil
		}
	}

	client, err := dialSSH(ctx, addr, s.sshConfig, s.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	done := make(chan error, 1)
	go func(c *ssh.Client) { done <- c.Conn.Wait() }(client)
	s.clients[addr] = &sshClientEntry{client: client, done: done}
	return client, nil
}

// CloseAll terminates all managed SSH client connections and clears the cache.
// Any errors encountered while closing clients are logged and aggregated.
func (s *SSHManager) CloseAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs error
	for addr, entry := range s.clients {
		if err := entry.client.Close(); err != nil {
			s.logger.Warn("client close error", zap.String("addr", addr), zap.Error(err))
			errs = multierr.Append(errs, fmt.Errorf("%s: %w", addr, err))
		}
		delete(s.clients, addr)
	}

	return errs
}

func getSSHAuthMethods(ctx context.Context, keyPath string, timeout time.Duration, logger *zap.Logger, parser func([]byte) (ssh.Signer, error)) ([]ssh.AuthMethod, error) {
	authMethods := []ssh.AuthMethod{}

	if keyPath != "" {
		signer, err := loadPrivateKey(parser, keyPath)
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

func loadPrivateKey(parse func([]byte) (ssh.Signer, error), keyPath string) (ssh.Signer, error) {
	info, err := os.Stat(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat private key: %w", err)
	}
	if info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("private key permissions %o are too open, want 0600", info.Mode().Perm())
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	signer, err := parse(keyData)
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
