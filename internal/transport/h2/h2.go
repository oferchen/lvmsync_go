package h2

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
	"net/http"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/http2"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func init() {
	transport.Register("h2", New)
}

// h2Sender issues an HTTP/2 POST request to the configured address and streams
// bytes from an io.Reader.
type h2Sender struct {
	addr    string
	tlsConf *tls.Config
	logger  *zap.Logger
}

// Send implements the transport.Sender interface.
func (s *h2Sender) Send(ctx context.Context, r io.Reader) error {
	client := &http.Client{Transport: &http2.Transport{TLSClientConfig: s.tlsConf}}
	url := fmt.Sprintf("https://%s", s.addr)
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return err
	}
	done := make(chan struct{})
	var n int64
	var copyErr error
	go func() {
		n, copyErr = io.Copy(pw, r)
		pw.CloseWithError(copyErr)
		close(done)
	}()
	s.logger.Info("h2 stream opened", zap.String("remote_addr", s.addr))
	resp, err := client.Do(req)
	if err != nil {
		_ = pw.Close()
		<-done
		s.logger.Error("h2 send error", zap.String("remote_addr", s.addr), zap.Error(err), zap.Int64("bytes_transferred", n))
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	<-done
	if copyErr != nil {
		s.logger.Error("h2 send error", zap.String("remote_addr", s.addr), zap.Error(copyErr), zap.Int64("bytes_transferred", n))
		return copyErr
	}
	s.logger.Info("h2 stream closed", zap.String("remote_addr", s.addr), zap.Int64("bytes_transferred", n))
	return nil
}

// h2Receiver listens for an incoming HTTP/2 request and writes bytes to an io.Writer.
type h2Receiver struct {
	ln      net.Listener
	tlsConf *tls.Config
	logger  *zap.Logger
}

// Receive implements the transport.Receiver interface.
func (r *h2Receiver) Receive(ctx context.Context, w io.Writer) error {
	mux := http.NewServeMux()
	done := make(chan struct{})
	mux.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		r.logger.Info("h2 stream opened", zap.String("remote_addr", req.RemoteAddr))
		n, err := io.Copy(w, req.Body)
		req.Body.Close()
		if err != nil {
			r.logger.Error("h2 receive error", zap.String("remote_addr", req.RemoteAddr), zap.Error(err), zap.Int64("bytes_transferred", n))
			http.Error(res, err.Error(), http.StatusInternalServerError)
		} else {
			r.logger.Info("h2 stream closed", zap.String("remote_addr", req.RemoteAddr), zap.Int64("bytes_transferred", n))
			res.WriteHeader(http.StatusOK)
		}
		close(done)
	})
	srv := &http.Server{Handler: mux, TLSConfig: r.tlsConf}
	http2.ConfigureServer(srv, &http2.Server{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(r.ln)
	}()
	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		_ = r.ln.Close()
		<-errCh
		return ctx.Err()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-done:
		_ = srv.Shutdown(context.Background())
		<-errCh
		return nil
	}
}

// Close releases the listener associated with the receiver.
func (r *h2Receiver) Close() error {
	if r.ln != nil {
		return r.ln.Close()
	}
	return nil
}

// New initializes an HTTP/2 sender and receiver bound to cfg.H2Port.
func New(cfg *config.Config, logger *zap.Logger) (transport.Sender, transport.Receiver, error) {
	cert, err := generateCert()
	if err != nil {
		return nil, nil, fmt.Errorf("generate cert: %w", err)
	}
	tlsConf := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{http2.NextProtoTLS}}
	ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", cfg.H2Port), tlsConf)
	if err != nil {
		return nil, nil, err
	}
	addr := ln.Addr().String()
	sender := &h2Sender{addr: addr, tlsConf: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http2.NextProtoTLS}}, logger: logger}
	receiver := &h2Receiver{ln: ln, tlsConf: tlsConf, logger: logger}
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
