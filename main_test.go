package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"lvmsync_go/common"
	"lvmsync_go/config"
)

// fakeSSHClient and fakeSSHSession implement the SSHClient/SSHSession
// interfaces without performing any network operations.
type fakeSSHClient struct{ session *fakeSSHSession }

func (c *fakeSSHClient) NewSession() (SSHSession, error) { return c.session, nil }
func (c *fakeSSHClient) Close() error                    { return nil }

type fakeSSHSession struct {
	stdin bytes.Buffer
}

func (s *fakeSSHSession) StdoutPipe() (io.Reader, error) { return strings.NewReader(""), nil }
func (s *fakeSSHSession) StderrPipe() (io.Reader, error) { return strings.NewReader(""), nil }
func (s *fakeSSHSession) StdinPipe() (io.WriteCloser, error) {
	return nopWriteCloser{&s.stdin}, nil
}
func (s *fakeSSHSession) Start(cmd string) error { return nil }
func (s *fakeSSHSession) Wait() error            { return nil }
func (s *fakeSSHSession) Close() error           { return nil }

type nopWriteCloser struct{ io.Writer }

func (n nopWriteCloser) Close() error { return nil }

// Test that runClientMode streams data to a remote session using a fake SSH client.
func TestRunClientModeWithFakeSSH(t *testing.T) {
	// Prepare a fake dump that runClientMode will stream.
	data := []byte("12345678")
	header := make([]byte, 12)
	binary.BigEndian.PutUint64(header[0:8], 0)
	binary.BigEndian.PutUint32(header[8:12], uint32(len(data)))
	dump := append([]byte(common.ProtocolVersion+"\n"), append(header, data...)...)

	// Override dumpChangesSequential to avoid disk operations.
	origDump := dumpChangesSequential
	dumpChangesSequential = func(cfg *config.Config, snapshot, source string, out io.Writer) error {
		_, err := out.Write(dump)
		return err
	}
	defer func() { dumpChangesSequential = origDump }()

	// Use a fake SSH client to capture what is written.
	session := &fakeSSHSession{}
	fakeClient := &fakeSSHClient{session: session}
	origNewClient := newSSHClient
	newSSHClient = func(host, user, keyPath string, port int, knownHostsPath string, verify bool, timeout, keepAliveInterval time.Duration, retries int) (SSHClient, error) {
		return fakeClient, nil
	}
	defer func() { newSSHClient = origNewClient }()

	cfg := &config.Config{Parallel: 1}
	if err := runClientMode(cfg, "snapshot", "remote:/dev/null"); err != nil {
		t.Fatalf("runClientMode returned error: %v", err)
	}

	if !bytes.Equal(session.stdin.Bytes(), dump) {
		t.Fatalf("unexpected stream data: %x", session.stdin.Bytes())
	}
}

// Test that runApplyMode applies a dump file to a destination using temporary files.
func TestRunApplyMode(t *testing.T) {
	data := []byte("abcdefgh")
	header := make([]byte, 12)
	binary.BigEndian.PutUint64(header[0:8], 0)
	binary.BigEndian.PutUint32(header[8:12], uint32(len(data)))
	dump := append([]byte(common.ProtocolVersion+"\n"), append(header, data...)...)

	applyFile := t.TempDir() + "/dump"
	if err := os.WriteFile(applyFile, dump, 0644); err != nil {
		t.Fatalf("failed to write dump file: %v", err)
	}
	destFile := t.TempDir() + "/dest"
	if err := os.WriteFile(destFile, make([]byte, len(data)), 0644); err != nil {
		t.Fatalf("failed to create dest file: %v", err)
	}

	cfg := &config.Config{Deduplication: false}
	if err := runApplyMode(cfg, applyFile, destFile); err != nil {
		t.Fatalf("runApplyMode returned error: %v", err)
	}

	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}
	if !bytes.Equal(got[:len(data)], data) {
		t.Fatalf("destination content mismatch: %q", got[:len(data)])
	}
}
