package quic

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"time"

	"go.uber.org/zap"

	quic "github.com/quic-go/quic-go"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

const alpn = "lvmsync"

// Transport implements a QUIC transport backed by quic-go with TLS 1.3,
// mutual authentication, ALPN negotiation, and datagram/stream support.
type Transport struct {
	serverTLS *tls.Config
	clientTLS *tls.Config
	qconf     *quic.Config
	logger    *zap.Logger
}

// Conn wraps a QUIC connection and stream to satisfy net.Conn and expose
// datagram APIs.
type Conn struct {
	qconn  quic.Connection
	stream quic.Stream
}

// listener adapts a quic.Listener to net.Listener by accepting a stream for
// each connection.
type listener struct {
	ql *quic.Listener
}

// New constructs a Transport using the provided TLS roots and client cert.
func New(cfg transport.Config) (transport.Interface, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if cfg.Roots == nil && !cfg.AllowInsecure {
		return nil, fmt.Errorf("tls roots are required unless AllowInsecure is set")
	}
	cert := cfg.ClientCert
	if len(cert.Certificate) == 0 {
		var err error
		cert, err = generateSelfSignedCert()
		if err != nil {
			return nil, err
		}
	}
	clientAuth := tls.RequireAndVerifyClientCert
	if cfg.AllowInsecure {
		clientAuth = tls.RequireAnyClientCert
	}
	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    cfg.Roots,
		ClientAuth:   clientAuth,
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{alpn},
	}
	clientTLS := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            cfg.Roots,
		InsecureSkipVerify: cfg.AllowInsecure,
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{alpn},
	}
	qconf := &quic.Config{EnableDatagrams: true}
	return &Transport{serverTLS: serverTLS, clientTLS: clientTLS, qconf: qconf, logger: cfg.Logger}, nil
}

func init() {
	if err := transport.Register("quic", New); err != nil {
		panic(err)
	}
}

func (t *Transport) Name() string { return "quic" }

// Dial dials a QUIC connection and opens a bidirectional stream.
func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	role := "client"
	t.logger.Info("dial_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
		zap.String("error", ""),
	)
	start := time.Now()
	qconn, err := quic.DialAddr(ctx, address, t.clientTLS, t.qconf)
	if err != nil {
		t.logger.Info("dial_end",
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.String("error", errString(err)),
		)
		return nil, err
	}
	stream, err := qconn.OpenStreamSync(ctx)
	t.logger.Info("dial_end",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("error", errString(err)),
	)
	if err != nil {
		qconn.CloseWithError(0, err.Error())
		return nil, err
	}
	return &Conn{qconn: qconn, stream: stream}, nil
}

// Listen starts a QUIC listener.
func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	role := "server"
	t.logger.Info("listen_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
		zap.String("error", ""),
	)
	start := time.Now()
	ql, err := quic.ListenAddr(address, t.serverTLS, t.qconf)
	t.logger.Info("listen_end",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("error", errString(err)),
	)
	if err != nil {
		return nil, err
	}
	return &listener{ql: ql}, nil
}

// Accept waits for the next connection and returns its first stream.
func (l *listener) Accept() (net.Conn, error) {
	qconn, err := l.ql.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	stream, err := qconn.AcceptStream(context.Background())
	if err != nil {
		qconn.CloseWithError(0, err.Error())
		return nil, err
	}
	return &Conn{qconn: qconn, stream: stream}, nil
}

func (l *listener) Close() error { return l.ql.Close() }

func (l *listener) Addr() net.Addr { return l.ql.Addr() }

// Negotiate performs the LVMSync handshake over the stream.
func (t *Transport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (peer common.Handshake, err error) {
	roleStr := roleString(role)
	address := conn.RemoteAddr().String()
	t.logger.Info("negotiate_start",
		zap.String("address", address),
		zap.String("role", roleStr),
		zap.Int64("duration_ms", 0),
		zap.String("error", ""),
	)
	start := time.Now()
	defer func() {
		t.logger.Info("negotiate_end",
			zap.String("address", address),
			zap.String("role", roleStr),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.String("error", errString(err)),
		)
	}()

	hs.Version = common.ProtocolVersion
	if hs.Endianness == "" {
		hs.Endianness = common.NativeEndianness()
	}
	switch role {
	case transport.Client:
		if err = common.WriteHandshake(conn, hs); err != nil {
			return peer, err
		}
		peer, err = common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			return peer, err
		}
		if err := common.ValidateHandshake(hs, peer); err != nil {
			return peer, err
		}
		return peer, nil
	case transport.Server:
		peer, err = common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			return peer, err
		}
		if err := common.ValidateHandshake(hs, peer); err != nil {
			return peer, err
		}
		if err = common.WriteHandshake(conn, hs); err != nil {
			return peer, err
		}
		return peer, nil
	default:
		return peer, nil
	}
}

// Datagram APIs.

// SendDatagram sends a datagram using the underlying QUIC connection.
func (c *Conn) SendDatagram(b []byte) error {
	return c.qconn.SendDatagram(b)
}

// ReceiveDatagram waits for the next datagram.
func (c *Conn) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	return c.qconn.ReceiveDatagram(ctx)
}

// net.Conn implementation.
func (c *Conn) Read(p []byte) (int, error)  { return c.stream.Read(p) }
func (c *Conn) Write(p []byte) (int, error) { return c.stream.Write(p) }
func (c *Conn) Close() error {
	_ = c.qconn.CloseWithError(0, "")
	return c.stream.Close()
}
func (c *Conn) LocalAddr() net.Addr  { return c.qconn.LocalAddr() }
func (c *Conn) RemoteAddr() net.Addr { return c.qconn.RemoteAddr() }
func (c *Conn) SetDeadline(t time.Time) error {
	if err := c.stream.SetDeadline(t); err != nil {
		return err
	}
	return nil
}
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.stream.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.stream.SetWriteDeadline(t) }

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func roleString(r transport.Role) string {
	switch r {
	case transport.Client:
		return "client"
	case transport.Server:
		return "server"
	default:
		return ""
	}
}

// generateSelfSignedCert creates a short-lived self-signed certificate.
func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return cert, nil
}
