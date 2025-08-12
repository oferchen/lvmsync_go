package dump

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/remote"
	"lvmsync_go/transfer"
)

func TestSetupSSHClient(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		dummy := &remote.SSHClient{}
		called := false
		newSSHClient = func(ctx context.Context, host, user, keyPath string, port int, knownHostsPath string, verify bool, timeout, keepAliveInterval time.Duration, retries int, logger *zap.Logger) (*remote.SSHClient, error) {
			called = true
			if host != "dest" {
				t.Fatalf("unexpected host %s", host)
			}
			return dummy, nil
		}
		defer func() { newSSHClient = remote.NewSSHClient }()

		client, cancel, err := SetupSSHClient(cfg, "dest", zap.NewNop())
		if err != nil {
			t.Fatalf("SetupSSHClient returned error: %v", err)
		}
		if client != dummy {
			t.Fatalf("expected dummy client")
		}
		if !called {
			t.Fatalf("newSSHClient was not called")
		}
		cancel()
	})

	t.Run("failure", func(t *testing.T) {
		newSSHClient = func(ctx context.Context, host, user, keyPath string, port int, knownHostsPath string, verify bool, timeout, keepAliveInterval time.Duration, retries int, logger *zap.Logger) (*remote.SSHClient, error) {
			return nil, errors.New("fail")
		}
		defer func() { newSSHClient = remote.NewSSHClient }()

		_, _, err := SetupSSHClient(cfg, "dest", zap.NewNop())
		if err == nil || !strings.Contains(err.Error(), "failed to create SSH client") {
			t.Fatalf("expected wrapped error, got %v", err)
		}
	})
}

type nopWriteCloser struct{ io.Writer }

func (n nopWriteCloser) Write(p []byte) (int, error) { return n.Writer.Write(p) }
func (n nopWriteCloser) Close() error                { return nil }

type mockPipeSession struct {
	stdoutReader io.Reader
	stderrReader io.Reader
	stdoutErr    error
	stderrErr    error
	stdinErr     error
	stdinWriter  io.WriteCloser
}

func (m *mockPipeSession) StdoutPipe() (io.Reader, error) {
	if m.stdoutReader == nil {
		m.stdoutReader = strings.NewReader("")
	}
	return m.stdoutReader, m.stdoutErr
}

func (m *mockPipeSession) StderrPipe() (io.Reader, error) {
	if m.stderrReader == nil {
		m.stderrReader = strings.NewReader("")
	}
	return m.stderrReader, m.stderrErr
}

func (m *mockPipeSession) StdinPipe() (io.WriteCloser, error) {
	if m.stdinErr != nil {
		return nil, m.stdinErr
	}
	if m.stdinWriter != nil {
		return m.stdinWriter, nil
	}
	return nopWriteCloser{Writer: io.Discard}, nil
}

func TestSetupSessionStreamsPipeErrors(t *testing.T) {
	tests := []struct {
		name string
		sess *mockPipeSession
		want string
	}{
		{
			name: "stdout pipe error",
			sess: &mockPipeSession{stdoutErr: errors.New("stdout")},
			want: "stdout pipe",
		},
		{
			name: "stderr pipe error",
			sess: &mockPipeSession{stderrErr: errors.New("stderr")},
			want: "stderr pipe",
		},
		{
			name: "stdin pipe error",
			sess: &mockPipeSession{stdinErr: errors.New("stdin")},
			want: "remote stdin",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := SetupSessionStreams(tc.sess)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s error, got %v", tc.want, err)
			}
		})
	}
}

type mockWriteCloser struct {
	io.Writer
	closeErr error
}

func (m *mockWriteCloser) Close() error { return m.closeErr }

func TestStreamToRemote(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.Parallel = 1

	orig := dumpChangesSequential
	t.Cleanup(func() { dumpChangesSequential = orig })

	tests := []struct {
		name     string
		dumpErr  error
		closeErr error
	}{
		{name: "success"},
		{name: "dump error", dumpErr: errors.New("dump")},
		{name: "close error", closeErr: errors.New("close")},
		{name: "both errors", dumpErr: errors.New("dump"), closeErr: errors.New("close")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dumpChangesSequential = func(_ *transfer.Transfer, c *config.Config, snapshot, origin string, out io.Writer) error {
				return tc.dumpErr
			}
			remoteStdin := &mockWriteCloser{Writer: io.Discard, closeErr: tc.closeErr}
			err := StreamToRemote(cfg, remoteStdin, "snap", "origin", zap.NewNop())
			if tc.dumpErr == nil && tc.closeErr == nil {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if tc.dumpErr != nil && !strings.Contains(err.Error(), tc.dumpErr.Error()) {
				t.Fatalf("error missing dump message: %v", err)
			}
			if tc.closeErr != nil && !strings.Contains(err.Error(), tc.closeErr.Error()) {
				t.Fatalf("error missing close message: %v", err)
			}
		})
	}
}

type mockWaitSession struct {
	err error
}

func (m *mockWaitSession) Wait() error { return m.err }

func TestWaitForRemoteCompletion(t *testing.T) {
	tests := []struct {
		name      string
		waitErr   error
		stdoutErr error
		stderrErr error
		want      string
	}{
		{name: "success"},
		{name: "wait error", waitErr: errors.New("wait"), want: "remote command error"},
		{name: "stdout error", stdoutErr: errors.New("out"), want: "stdout copy error"},
		{name: "stderr error", stderrErr: errors.New("err"), want: "stderr copy error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdoutCh := make(chan error, 1)
			stderrCh := make(chan error, 1)
			if tc.stdoutErr != nil {
				stdoutCh <- tc.stdoutErr
			} else {
				stdoutCh <- nil
			}
			if tc.stderrErr != nil {
				stderrCh <- tc.stderrErr
			} else {
				stderrCh <- nil
			}
			err := WaitForRemoteCompletion(&mockWaitSession{err: tc.waitErr}, stdoutCh, stderrCh)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s, got %v", tc.want, err)
			}
		})
	}
}
