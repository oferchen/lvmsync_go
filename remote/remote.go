// remote/remote.go
package remote

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func NewSSHClient(host, user, keyPath string, port int, knownHostsPath string, verify bool) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod
	if keyPath != "" {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read SSH key file: %v", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH key: %v", err)
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
	var hostKeyCallback ssh.HostKeyCallback
	if verify {
		var err error
		hostKeyCallback, err = knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create knownhosts callback: %v", err)
		}
	} else {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	}
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	return ssh.Dial("tcp", addr, config)
}

func ValidateRemoteCommand(client *ssh.Client, remoteCmd string) error {
	tokens := strings.Fields(remoteCmd)
	if len(tokens) == 0 {
		return fmt.Errorf("remote command is empty")
	}
	cmd := tokens[0]
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session for validation: %v", err)
	}
	defer session.Close()
	if err := session.Run(fmt.Sprintf("command -v %s >/dev/null 2>&1", cmd)); err != nil {
		return fmt.Errorf("remote command %s not found or not executable: %v", cmd, err)
	}
	return nil
}

func RunRemoteScript(client *ssh.Client, script string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session for script: %v", err)
	}
	defer session.Close()
	Logger.Info("Running remote script", zap.String("script", script))
	return session.Run(script)
}

var Logger *zap.Logger

func SetLogger(logger *zap.Logger) {
	Logger = logger
}
