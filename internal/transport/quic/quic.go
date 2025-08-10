package quic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"time"

	"go.uber.org/zap"

	q "github.com/quic-go/quic-go"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func init() {
	transport.Register("quic", New)
}

// quicSender dials a remote QUIC endpoint and streams bytes from an io.Reader.
type quicSender struct {
	addr     string
	tlsConf  *tls.Config
	quicConf *q.Config
	logger   *zap.Logger
	cc       string
}

// Send implements the transport.Sender interface.
func (s *quicSender) Send(ctx context.Context, r io.Reader) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	sess, err := q.DialAddr(ctx, s.addr, s.tlsConf, s.quicConf)
	if err != nil {
		s.logger.Error("quic dial error", zap.String("remote_addr", s.addr), zap.Error(err), zap.String("cc", s.cc))
		return err
	}
	s.logger.Info("quic session established", zap.String("remote_addr", s.addr), zap.String("cc", s.cc))

	stream, err := sess.OpenStreamSync(ctx)
	if err != nil {
		sess.CloseWithError(0, "")
		s.logger.Error("quic stream open error", zap.String("remote_addr", s.addr), zap.Error(err), zap.String("cc", s.cc))
		return err
	}
	s.logger.Info("quic stream opened", zap.String("remote_addr", s.addr))
	n, err := io.Copy(stream, r)
	if err != nil {
		stream.CancelWrite(0)
		sess.CloseWithError(0, "")
		s.logger.Error("quic send error", zap.String("remote_addr", s.addr), zap.Error(err), zap.Int64("bytes_transferred", n), zap.String("cc", s.cc))
		return err
	}
	if err := stream.Close(); err != nil {
		sess.CloseWithError(0, "")
		s.logger.Warn("quic stream close error", zap.String("remote_addr", s.addr), zap.Error(err))
		return err
	}
	s.logger.Info("quic stream closed", zap.String("remote_addr", s.addr), zap.Int64("bytes_transferred", n))
	<-sess.Context().Done()
	return nil
}

// quicReceiver listens for incoming QUIC connections and writes bytes to an io.Writer.
type quicReceiver struct {
	ln     *q.Listener
	logger *zap.Logger
	cc     string
}

// Receive implements the transport.Receiver interface.
func (r *quicReceiver) Receive(ctx context.Context, w io.Writer) error {
	conn, err := r.ln.Accept(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	r.logger.Info("quic session accepted", zap.String("remote_addr", conn.RemoteAddr().String()), zap.String("cc", r.cc))
	defer conn.CloseWithError(0, "")

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r.logger.Error("quic accept stream error", zap.Error(err))
		return err
	}
	r.logger.Info("quic stream accepted", zap.String("remote_addr", conn.RemoteAddr().String()))
	n, err := io.Copy(w, stream)
	if err != nil {
		stream.CancelRead(0)
		r.logger.Error("quic receive error", zap.Error(err), zap.Int64("bytes_transferred", n), zap.String("cc", r.cc))
		return err
	}
	r.logger.Info("quic stream closed", zap.String("remote_addr", conn.RemoteAddr().String()), zap.Int64("bytes_transferred", n))
	return nil
}

// Close releases the underlying listener.
func (r *quicReceiver) Close() error {
	if r.ln != nil {
		return r.ln.Close()
	}
	return nil
}

// New initializes QUIC sender and receiver according to configuration.
func New(cfg *config.Config, logger *zap.Logger) (transport.Sender, transport.Receiver, error) {
	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"lvmsync"}}
	quicConf := &q.Config{Allow0RTT: false}

	var sender transport.Sender = transport.NopSender{}
	if cfg.QUICConnect != "" {
		sender = &quicSender{addr: cfg.QUICConnect, tlsConf: tlsConf, quicConf: quicConf, logger: logger, cc: cfg.QUICCongestionControl}
	}

	var receiver transport.Receiver = transport.NopReceiver{}
	if cfg.QUICListen != "" {
		cert, err := generateCert()
		if err != nil {
			return nil, nil, err
		}
		srvTLS := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"lvmsync"}}
		ln, err := q.ListenAddr(cfg.QUICListen, srvTLS, quicConf)
		if err != nil {
			return nil, nil, err
		}
		receiver = &quicReceiver{ln: ln, logger: logger, cc: cfg.QUICCongestionControl}
	}

	return sender, receiver, nil
}

func generateCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return tls.X509KeyPair(certPEM, keyPEM)
}
