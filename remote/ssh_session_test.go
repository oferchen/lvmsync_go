package remote

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	remotetest "lvmsync_go/remote/testutil"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func TestLoadPrivateKey(t *testing.T) {
	keyFile := remotetest.CreateTempKey(t)
	if _, err := readPrivateKey(keyFile); err != nil {
		t.Fatalf("loadPrivateKey valid: %v", err)
	}
	if _, err := readPrivateKey(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatalf("expected error for missing key")
	}
}

func TestLoadHostPublicKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	file := filepath.Join(t.TempDir(), "host_key.pub")
	if err := os.WriteFile(file, pub.Marshal(), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := readHostPublicKey(file)
	if err != nil {
		t.Fatalf("loadHostPublicKey valid: %v", err)
	}
	if !bytes.Equal(loaded.Marshal(), pub.Marshal()) {
		t.Fatalf("loaded key mismatch")
	}
	if _, err := readHostPublicKey(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatalf("expected error for missing host key")
	}
}

func TestRunSSHCommandSuccess(t *testing.T) {
	srv := remotetest.NewMockSSHServerWithChannel(t, func(cmd string, ch ssh.Channel) int {
		ch.Write([]byte("out"))          //nolint:errcheck
		ch.Stderr().Write([]byte("err")) //nolint:errcheck
		return 0
	})
	defer srv.Close()

	keyFile := remotetest.CreateTempKey(t)
	hostKey := filepath.Join(t.TempDir(), "host_key.pub")
	if err := os.WriteFile(hostKey, srv.PublicKey.Marshal(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	host, portStr, err := net.SplitHostPort(srv.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := RunSSHCommand(context.Background(), zap.NewNop(), host, "test", keyFile, hostKey, port, "cmd", time.Second, &stdout, &stderr); err != nil {
		t.Fatalf("RunSSHCommand: %v", err)
	}
	if stdout.String() != "out" {
		t.Fatalf("unexpected stdout %q", stdout.String())
	}
	if stderr.String() != "err" {
		t.Fatalf("unexpected stderr %q", stderr.String())
	}
}

func TestRunSSHCommandFailure(t *testing.T) {
	srv := remotetest.NewMockSSHServer(t, func(cmd string) int { return 1 })
	defer srv.Close()

	keyFile := remotetest.CreateTempKey(t)
	hostKey := filepath.Join(t.TempDir(), "host_key.pub")
	if err := os.WriteFile(hostKey, srv.PublicKey.Marshal(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	host, portStr, err := net.SplitHostPort(srv.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	if err := RunSSHCommand(context.Background(), zap.NewNop(), host, "test", keyFile, hostKey, port, "cmd", time.Second, nil, nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunSSHCommandTimeout(t *testing.T) {
	srv := remotetest.NewMockSSHServer(t, func(cmd string) int {
		time.Sleep(200 * time.Millisecond)
		return 0
	})
	defer srv.Close()

	keyFile := remotetest.CreateTempKey(t)
	hostKey := filepath.Join(t.TempDir(), "host_key.pub")
	if err := os.WriteFile(hostKey, srv.PublicKey.Marshal(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	host, portStr, err := net.SplitHostPort(srv.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := RunSSHCommand(ctx, zap.NewNop(), host, "test", keyFile, hostKey, port, "cmd", time.Second, nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}
