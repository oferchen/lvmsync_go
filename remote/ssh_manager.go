// remote/ssh_manager.go
package remote

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type SSHManager struct {
	mu        sync.Mutex
	clients   map[string]*ssh.Client
	sshConfig *ssh.ClientConfig
	timeout   time.Duration
}

func NewSSHManager(user, keyPath string, timeout time.Duration, knownHostsPath string, verify bool) (*SSHManager, error) {
	authMethods, err := getSSHAuthMethods(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authentication: %w", err)
	}

	hostKeyCallback, err := getHostKeyCallback(knownHostsPath, verify)
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

func (s *SSHManager) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for addr, client := range s.clients {
		client.Close()
		delete(s.clients, addr)
	}
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

func getHostKeyCallback(knownHostsPath string, verify bool) (ssh.HostKeyCallback, error) {
	if verify {
		return knownhosts.New(knownHostsPath)
	}
	return ssh.InsecureIgnoreHostKey(), nil
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

	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers), nil
}
