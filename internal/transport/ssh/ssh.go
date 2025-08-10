package ssh

import (
	"context"
	"io"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
	"lvmsync_go/remote"
)

var logger = zap.NewNop()

// SetLogger assigns the package-wide logger.
func SetLogger(l *zap.Logger) {
	if l != nil {
		logger = l
	}
}

func init() {
	transport.Register("ssh", New)
}

type sshSender struct {
	client *remote.SSHClient
	host   string
	port   int
	logger *zap.Logger
}

type sshReceiver struct {
	client *remote.SSHClient
	host   string
	port   int
	logger *zap.Logger
}

// New wraps the existing SSH client as a transport.
func New(cfg *config.Config) (transport.Sender, transport.Receiver, error) {
	host := "localhost"
	logger.Info("ssh connection attempt", zap.String("host", host), zap.Int("port", cfg.SSHPort))
	client, err := remote.NewSSHClient(host, cfg.SSHUser, cfg.SSHKeyPath, cfg.SSHPort, cfg.KnownHosts, cfg.StrictHostKeyCheck, cfg.SSHTimeout, cfg.SSHKeepAliveInterval, cfg.MaxRetries, logger)
	if err != nil {
		logger.Error("ssh authentication failed", zap.String("host", host), zap.Int("port", cfg.SSHPort), zap.Error(err))
		return nil, nil, err
	}
	logger.Info("ssh connection established", zap.String("host", host), zap.Int("port", cfg.SSHPort))
	sender := &sshSender{client: client, host: host, port: cfg.SSHPort, logger: logger}
	receiver := &sshReceiver{client: client, host: host, port: cfg.SSHPort, logger: logger}
	return sender, receiver, nil
}

func (s *sshSender) Send(ctx context.Context, r io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	session, err := s.client.NewSession()
	if err != nil {
		s.logger.Error("ssh session start error", zap.String("host", s.host), zap.Int("port", s.port), zap.Error(err))
		return err
	}
	defer func() { _ = session.Close() }()
	stdin, err := session.StdinPipe()
	if err != nil {
		s.logger.Error("ssh stdin pipe error", zap.String("host", s.host), zap.Int("port", s.port), zap.Error(err))
		return err
	}
	done := make(chan struct{})
	var n int64
	var copyErr error
	go func() {
		n, copyErr = io.Copy(stdin, r)
		if closeErr := stdin.Close(); closeErr != nil && copyErr == nil {
			copyErr = closeErr
		}
		close(done)
	}()
	if err := session.Start("sink"); err != nil {
		s.logger.Error("ssh session command error", zap.String("host", s.host), zap.Int("port", s.port), zap.Error(err))
		<-done
		return err
	}
	select {
	case <-ctx.Done():
		_ = session.Close()
		<-done
		s.logger.Info("ssh send stream closed", zap.String("host", s.host), zap.Int("port", s.port), zap.Int64("bytes_transferred", n))
		return ctx.Err()
	case <-done:
	}
	if waitErr := session.Wait(); waitErr != nil && copyErr == nil {
		copyErr = waitErr
	}
	if copyErr != nil {
		s.logger.Error("ssh send error", zap.String("host", s.host), zap.Int("port", s.port), zap.Error(copyErr), zap.Int64("bytes_transferred", n))
	} else {
		s.logger.Info("ssh send stream closed", zap.String("host", s.host), zap.Int("port", s.port), zap.Int64("bytes_transferred", n))
	}
	return copyErr
}

func (r *sshReceiver) Receive(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	session, err := r.client.NewSession()
	if err != nil {
		r.logger.Error("ssh session start error", zap.String("host", r.host), zap.Int("port", r.port), zap.Error(err))
		return err
	}
	defer func() { _ = session.Close() }()
	stdout, err := session.StdoutPipe()
	if err != nil {
		r.logger.Error("ssh stdout pipe error", zap.String("host", r.host), zap.Int("port", r.port), zap.Error(err))
		return err
	}
	done := make(chan struct{})
	var n int64
	var copyErr error
	go func() {
		n, copyErr = io.Copy(w, stdout)
		close(done)
	}()
	if err := session.Start("source"); err != nil {
		r.logger.Error("ssh session command error", zap.String("host", r.host), zap.Int("port", r.port), zap.Error(err))
		<-done
		return err
	}
	select {
	case <-ctx.Done():
		_ = session.Close()
		<-done
		r.logger.Info("ssh receive stream closed", zap.String("host", r.host), zap.Int("port", r.port), zap.Int64("bytes_transferred", n))
		return ctx.Err()
	case <-done:
	}
	if waitErr := session.Wait(); waitErr != nil && copyErr == nil {
		copyErr = waitErr
	}
	if copyErr != nil {
		r.logger.Error("ssh receive error", zap.String("host", r.host), zap.Int("port", r.port), zap.Error(copyErr), zap.Int64("bytes_transferred", n))
	} else {
		r.logger.Info("ssh receive stream closed", zap.String("host", r.host), zap.Int("port", r.port), zap.Int64("bytes_transferred", n))
	}
	return copyErr

}
