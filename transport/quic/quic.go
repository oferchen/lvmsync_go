package quic

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
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
	ql  *quic.Listener
	ctx context.Context
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
	if len(cert.Certificate) == 0 && !cfg.AllowInsecure {
		return nil, fmt.Errorf("client certificate is required unless AllowInsecure is set")
	}
	clientAuth := tls.RequireAndVerifyClientCert
	if cfg.AllowInsecure {
		cfg.Logger.Warn("allow_insecure_enabled", zap.String("transport", "quic"))
		clientAuth = tls.RequireAnyClientCert
	}
	serverTLS := &tls.Config{
		ClientCAs:  cfg.Roots,
		ClientAuth: clientAuth,
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{alpn},
	}
	clientTLS := &tls.Config{
		RootCAs:            cfg.Roots,
		InsecureSkipVerify: cfg.AllowInsecure,
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{alpn},
	}
	if len(cert.Certificate) > 0 {
		serverTLS.Certificates = []tls.Certificate{cert}
		clientTLS.Certificates = []tls.Certificate{cert}
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
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	stream, err := qconn.OpenStreamSync(ctx)
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.Error(err),
	}
	if err != nil {
		t.logger.Error("dial_end", fields...)
		qconn.CloseWithError(0, err.Error())
		return nil, err
	}
	fields = []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("error", ""),
	}
	t.logger.Info("dial_end", fields...)
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
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.Error(err),
	}
	if err != nil {
		t.logger.Error("listen_end", fields...)
		return nil, err
	}
	fields = []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("error", ""),
	}
	t.logger.Info("listen_end", fields...)
	return &listener{ql: ql, ctx: ctx}, nil
}

// Accept waits for the next connection and returns its first stream.
//
// It uses the context passed to Listen, allowing callers to cancel the
// pending accept via context cancellation.
func (l *listener) Accept() (net.Conn, error) {
	qconn, err := l.ql.Accept(l.ctx)
	if err != nil {
		return nil, err
	}
	stream, err := qconn.AcceptStream(l.ctx)
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
	)
	start := time.Now()
	defer func() {
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", roleStr),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		}
		if err != nil {
			fields = append(fields, zap.Error(err))
			t.logger.Error("negotiate_end", fields...)
		} else {
			fields = append(fields,
				zap.String("dedup_mode", hs.DedupMode),
				zap.Int("block_size_bytes", hs.BlockSize),
				zap.String("compress", hs.Compress),
				zap.Int("compress_level", hs.CompressLevel),
				zap.String("digest", hs.Digest),
				zap.String("resume_token", hs.ResumeToken),
				zap.Bool("checksum", hs.Checksum),
				zap.Bool("checksum_dedup", hs.ChecksumDedup),
				zap.String("endianness", hs.Endianness),
				zap.Bool("odirect", hs.ODirect),
				zap.Int("max_inflight", hs.MaxInFlight),
				zap.Int("cdc_min", hs.CDCMin),
				zap.Int("cdc_avg", hs.CDCAvg),
				zap.Int("cdc_max", hs.CDCMax),
				zap.String("transport", hs.Transport),
				zap.String("alpn", hs.ALPN),
				zap.String("tls_version", hs.TLSVersion),
			)
			t.logger.Info("negotiate_end", fields...)
		}
	}()

	if qc, ok := conn.(*Conn); ok {
		state := qc.qconn.ConnectionState()
		negotiatedALPN := state.TLS.NegotiatedProtocol
		negotiatedVersion := tlsVersionString(state.TLS.Version)
		if hs.ALPN != "" && negotiatedALPN != "" && hs.ALPN != negotiatedALPN {
			return peer, fmt.Errorf("alpn mismatch: %s", negotiatedALPN)
		}
		if hs.TLSVersion != "" && negotiatedVersion != "" && hs.TLSVersion != negotiatedVersion {
			return peer, fmt.Errorf("tls version mismatch: %s", negotiatedVersion)
		}
		hs.ALPN = negotiatedALPN
		hs.TLSVersion = negotiatedVersion
	}

	hs.Version = common.ProtocolVersion
	if hs.Endianness == "" {
		hs.Endianness = common.NativeEndianness()
	}
	switch role {
	case transport.Client:
		if err = setDeadline(ctx, conn); err != nil {
			return peer, err
		}
		if err = common.WriteHandshake(conn, hs); err != nil {
			clearDeadline(conn)
			return peer, err
		}
		clearDeadline(conn)

		if err = setDeadline(ctx, conn); err != nil {
			return peer, err
		}
		peer, err = common.ReadHandshake(bufio.NewReader(conn))
		clearDeadline(conn)
		if err != nil {
			return peer, err
		}
		if err := common.ValidateHandshake(hs, peer); err != nil {
			return peer, err
		}
		return peer, nil
	case transport.Server:
		if err = setDeadline(ctx, conn); err != nil {
			return peer, err
		}
		peer, err = common.ReadHandshake(bufio.NewReader(conn))
		clearDeadline(conn)
		if err != nil {
			return peer, err
		}
		if err := common.ValidateHandshake(hs, peer); err != nil {
			return peer, err
		}
		if err = setDeadline(ctx, conn); err != nil {
			return peer, err
		}
		if err = common.WriteHandshake(conn, hs); err != nil {
			clearDeadline(conn)
			return peer, err
		}
		clearDeadline(conn)
		return peer, nil
	default:
		return peer, nil
	}
}

func setDeadline(ctx context.Context, conn net.Conn) error {
	if dl, ok := ctx.Deadline(); ok {
		return conn.SetDeadline(dl)
	}
	return nil
}

func clearDeadline(conn net.Conn) {
	_ = conn.SetDeadline(time.Time{})
}

func tlsVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return ""
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
