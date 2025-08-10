package tcp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func init() {
	transport.Register("tcp+tls", New)
}

// tcpSender establishes a TLS connection to the configured address and
// streams bytes from an io.Reader.
type tcpSender struct {
	addr    string
	tlsConf *tls.Config
	logger  *zap.Logger
}

// Send implements the transport.Sender interface.
func (s *tcpSender) Send(ctx context.Context, r io.Reader) error {
	d := &net.Dialer{}
	conn, err := tls.DialWithDialer(d, "tcp", s.addr, s.tlsConf)
	if err != nil {
		s.logger.Error("tcp dial error", zap.String("remote_addr", s.addr), zap.Error(err))
		return err
	}
	remoteAddr := conn.RemoteAddr().String()
	s.logger.Info("tcp connection opened", zap.String("remote_addr", remoteAddr))
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	n, copyErr := io.Copy(conn, r)
	if closeErr := conn.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	close(done)
	if copyErr != nil {
		s.logger.Error("tcp send error", zap.String("remote_addr", remoteAddr), zap.Error(copyErr), zap.Int64("bytes_transferred", n))
		return copyErr
	}
	s.logger.Info("tcp connection closed", zap.String("remote_addr", remoteAddr), zap.Int64("bytes_transferred", n))
	return nil
}

// tcpReceiver listens for an incoming TLS connection and writes bytes to an io.Writer.
type tcpReceiver struct {
	ln     net.Listener
	logger *zap.Logger
}

// Receive implements the transport.Receiver interface.
func (r *tcpReceiver) Receive(ctx context.Context, w io.Writer) error {
	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := r.ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		connCh <- c
	}()
	var conn net.Conn
	select {
	case <-ctx.Done():
		_ = r.ln.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	case conn = <-connCh:
	}
	remoteAddr := conn.RemoteAddr().String()
	r.logger.Info("tcp connection opened", zap.String("remote_addr", remoteAddr))
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	n, copyErr := io.Copy(w, conn)
	if closeErr := conn.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	close(done)
	if copyErr != nil {
		r.logger.Error("tcp receive error", zap.String("remote_addr", remoteAddr), zap.Error(copyErr), zap.Int64("bytes_transferred", n))
		return copyErr
	}
	r.logger.Info("tcp connection closed", zap.String("remote_addr", remoteAddr), zap.Int64("bytes_transferred", n))
	return nil
}

// Close releases the listener associated with the receiver.
func (r *tcpReceiver) Close() error {
	if r.ln != nil {
		return r.ln.Close()
	}
	return nil
}

// New initializes a TCP+TLS sender and receiver bound to cfg.TCPPort.
func New(cfg *config.Config, logger *zap.Logger) (transport.Sender, transport.Receiver, error) {
	cert, err := generateCert()
	if err != nil {
		return nil, nil, fmt.Errorf("generate cert: %w", err)
	}
	ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", cfg.TCPPort), &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		return nil, nil, err
	}
	addr := ln.Addr().String()
	sender := &tcpSender{addr: addr, tlsConf: &tls.Config{InsecureSkipVerify: true}, logger: logger}
	receiver := &tcpReceiver{ln: ln, logger: logger}
	return sender, receiver, nil
}

func generateCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return tls.X509KeyPair(certPEM, keyPEM)
}
