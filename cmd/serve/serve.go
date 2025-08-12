package serve

import (
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"

	q "github.com/quic-go/quic-go"

	"lvmsync_go/config"
	qn "lvmsync_go/quic"
)

// acceptFunc allows tests to override the listener and stream acceptance logic.
// The context controls cancellation for both the listener and the stream accepts
// and must be honored by implementations.
var acceptFunc = func(ctx context.Context, cfg *config.Config) (io.ReadWriteCloser, error) {
	tlsConf, err := qn.NewTLSConfig(qn.Config{
		TLSCert:       cfg.TLSCert,
		TLSKey:        cfg.TLSKey,
		CACert:        cfg.CACert,
		AllowInsecure: cfg.AllowInsecure,
	}, true)
	if err != nil {
		return nil, err
	}

	listenCtx, listenCancel := context.WithTimeout(ctx, cfg.ServeAcceptTimeout)
	defer listenCancel()
	l, err := q.ListenAddr(cfg.ServeListen, tlsConf, qn.NewQUICConfig(0))
	if err != nil {
		return nil, err
	}
	if err := listenCtx.Err(); err != nil {
		_ = l.Close()
		return nil, err
	}

	connCtx, connCancel := context.WithTimeout(ctx, cfg.ServeAcceptTimeout)
	conn, err := l.Accept(connCtx)
	connCancel()
	if err != nil {
		_ = l.Close()
		return nil, err
	}

	streamCtx, streamCancel := context.WithTimeout(ctx, cfg.ServeAcceptTimeout)
	stream, err := conn.AcceptStream(streamCtx)
	streamCancel()
	if err != nil {
		_ = conn.CloseWithError(0, "")
		_ = l.Close()
		return nil, err
	}
	return &quicStream{ReadWriteCloser: stream, conn: conn, listener: l}, nil
}

// quicStream wraps a QUIC stream and ensures the QUIC connection and listener
// are closed when the stream is closed.
type connCloser interface {
	CloseWithError(code q.ApplicationErrorCode, msg string) error
}

type listenerCloser interface {
	Close() error
}

type quicStream struct {
	io.ReadWriteCloser
	conn     connCloser
	listener listenerCloser
}

// Close closes the stream, connection, and listener, ignoring connection
// errors so shutdown paths can proceed.
func (qs *quicStream) Close() error {
	_ = qs.conn.CloseWithError(0, "")
	_ = qs.listener.Close()
	return qs.ReadWriteCloser.Close()
}

// Run starts the QUIC server, negotiates parameters, and enforces the transfer policy.
// The context allows callers to cancel pending accept operations and shut down gracefully.
func Run(ctx context.Context, cfg *config.Config, logger *zap.Logger) error {
	stream, err := acceptFunc(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := stream.Close(); cerr != nil {
			logger.Warn("serve: stream close", zap.Error(cerr))
		}
	}()

	expected := qn.Negotiation{
		Protocol:  cfg.ServeProtocol,
		Algorithm: cfg.ServeAlgorithm,
		TestSpace: cfg.ServeTestSpace,
	}
	if err := qn.Negotiate(stream, expected); err != nil {
		return err
	}
	if cfg.ServePolicy != "" && cfg.ServePolicy != "accept" {
		return fmt.Errorf("transfer policy %s not accepted", cfg.ServePolicy)
	}
	logger.Info("serve: negotiation complete")
	return nil
}
