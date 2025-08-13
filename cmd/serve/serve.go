package serve

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"strings"
	"time"

	quic "github.com/quic-go/quic-go"
	"go.uber.org/zap"

	"lvmsync_go/config"
)

// generateTLSConfig creates an ephemeral certificate for QUIC.
func generateTLSConfig(proto string) (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "lvmsync"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	tlsCert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}, NextProtos: []string{proto}}, nil
}

// Run starts a basic gRPC/QUIC server honoring serve_* flags and shuts down when the context is canceled.
func Run(ctx context.Context, cfg *config.Config, logger *zap.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		if err := logger.Sync(); err != nil {
			logger.Error("logger sync error", zap.Error(err))
		}
	}()

	if cfg.ServePolicy != "accept" {
		logger.Error("serve policy rejects transfers", zap.String("serve_policy", cfg.ServePolicy))
		return fmt.Errorf("serve policy %s rejects transfers", cfg.ServePolicy)
	}

	tlsConf, err := generateTLSConfig(cfg.ServeProtocol)
	if err != nil {
		logger.Error("tls config", zap.Error(err))
		return err
	}

	ln, err := quic.ListenAddr(cfg.ServeListen, tlsConf, nil)
	if err != nil {
		logger.Error("listen", zap.String("listen_addr", cfg.ServeListen), zap.Error(err))
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()
	logger.Info("listening", zap.String("listen_addr", cfg.ServeListen))

	acceptCtx, cancelAccept := context.WithTimeout(ctx, cfg.ServeAcceptTimeout)
	conn, err := ln.Accept(acceptCtx)
	cancelAccept()
	if err != nil {
		logger.Error("accept connection", zap.Error(err))
		return fmt.Errorf("accept connection: %w", err)
	}
	logger.Info("connection accepted", zap.String("remote_addr", conn.RemoteAddr().String()))

	if conn.ConnectionState().TLS.NegotiatedProtocol != cfg.ServeProtocol {
		conn.CloseWithError(0, "protocol mismatch")
		return fmt.Errorf("protocol mismatch")
	}

	streamCtx, cancelStream := context.WithTimeout(ctx, cfg.ServeAcceptTimeout)
	stream, err := conn.AcceptStream(streamCtx)
	cancelStream()
	if err != nil {
		conn.CloseWithError(0, "stream error")
		logger.Error("accept stream", zap.Error(err))
		return fmt.Errorf("accept stream: %w", err)
	}

	reader := bufio.NewReader(stream)
	line, err := reader.ReadString('\n')
	if err != nil {
		conn.CloseWithError(0, "handshake read error")
		logger.Error("read handshake", zap.Error(err))
		return fmt.Errorf("read handshake: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(line), "|")
	alg := ""
	test := ""
	if len(parts) > 0 {
		alg = parts[0]
	}
	if len(parts) > 1 {
		test = parts[1]
	}
	if alg != cfg.ServeAlgorithm || test != cfg.ServeTestSpace {
		conn.CloseWithError(0, "handshake mismatch")
		logger.Error("handshake mismatch", zap.String("algorithm", alg), zap.String("test_space", test))
		return fmt.Errorf("handshake mismatch")
	}
	logger.Info("handshake complete", zap.String("algorithm", alg), zap.String("test_space", test))

	<-ctx.Done()
	conn.CloseWithError(0, "server shutdown")
	logger.Info("shutdown", zap.Error(ctx.Err()))
	return nil
}
