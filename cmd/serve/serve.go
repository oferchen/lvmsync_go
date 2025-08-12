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
// The provided context controls cancellation for listener and stream accepts.
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
	l, err := q.ListenAddr(cfg.ServeListen, tlsConf, qn.NewQUICConfig())
	if err != nil {
		return nil, err
	}
	conn, err := l.Accept(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// Run starts the QUIC server, negotiates parameters, and enforces the transfer policy.
// The context allows callers to cancel pending accept operations and shut down gracefully.
func Run(ctx context.Context, cfg *config.Config, logger *zap.Logger) error {
	stream, err := acceptFunc(ctx, cfg)
	if err != nil {
		return err
	}
	defer stream.Close()

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
