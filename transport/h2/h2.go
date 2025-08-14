package h2

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

// Transport implements HTTP/2 over TLS1.3 with mutual authentication.
type Transport struct {
	clientConf *tls.Config
	serverConf *tls.Config
	logger     *zap.Logger
}

// New returns a new Transport.
func New(cfg transport.Config) (transport.Interface, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if cfg.Roots == nil {
		return nil, fmt.Errorf("tls roots are required")
	}
	if len(cfg.ServerCert.Certificate) == 0 {
		return nil, fmt.Errorf("server certificate is required")
	}
	clientConf := &tls.Config{
		RootCAs:    cfg.Roots,
		NextProtos: []string{"h2"},
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
	}
	if len(cfg.ClientCert.Certificate) != 0 {
		clientConf.Certificates = []tls.Certificate{cfg.ClientCert}
	}
	serverConf := &tls.Config{
		Certificates: []tls.Certificate{cfg.ServerCert},
		ClientCAs:    cfg.Roots,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{"h2"},
	}
	return &Transport{clientConf: clientConf, serverConf: serverConf, logger: cfg.Logger}, nil
}

func init() {
	if err := transport.Register("h2", New); err != nil {
		panic(err)
	}
}

func (t *Transport) Name() string { return "h2" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	role := "client"
	t.logger.Info("dial_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	d := net.Dialer{}
	if dl, ok := ctx.Deadline(); ok {
		d.Deadline = dl
	}
	conn, err := tls.DialWithDialer(&d, "tcp", address, t.clientConf)
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	t.logger.Info("dial_end", fields...)
	return conn, err
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	role := "server"
	t.logger.Info("listen_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	ln, err := net.Listen("tcp", address)
	if err == nil {
		ln = tls.NewListener(ln, t.serverConf)
	}
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	t.logger.Info("listen_end", fields...)
	return ln, err
}

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
		}
		t.logger.Info("negotiate_end", fields...)
	}()

	if tlsConn, ok := conn.(*tls.Conn); ok {
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return peer, err
		}
		state := tlsConn.ConnectionState()
		hs.ALPN = state.NegotiatedProtocol
		hs.TLSVersion = tlsVersionString(state.Version)
	}
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
