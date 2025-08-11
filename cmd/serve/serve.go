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
var acceptFunc = func(cfg *config.Config) (io.ReadWriteCloser, error) {
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
	conn, err := l.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// Run starts the QUIC server, negotiates parameters, and enforces the transfer policy.
func Run(cfg *config.Config, logger *zap.Logger) error {
	stream, err := acceptFunc(cfg)
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
