package tcp_tls

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/common"
	"lvmsync_go/internal/logging"
	"lvmsync_go/transport"
)

const (
	defaultDialTimeout = 5 * time.Second
	alpn               = "lvmsync"
)

// Transport implements TLS over TCP.
type Transport struct {
	serverConf *tls.Config
	clientConf *tls.Config
	logger     *zap.Logger
}

// New creates a Transport using provided TLS roots, server certificate, and
// client certificate. Logger is optional; a no-op logger is used when nil.
func New(cfg transport.Config) (transport.Interface, error) {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	if cfg.Roots == nil && !cfg.AllowInsecure {
		return nil, fmt.Errorf("tls roots are required unless AllowInsecure is set")
	}
	if len(cfg.ServerCert.Certificate) == 0 && !cfg.AllowInsecure {
		return nil, fmt.Errorf("server certificate is required unless AllowInsecure is set")
	}
	if len(cfg.ClientCert.Certificate) == 0 && !cfg.AllowInsecure {
		return nil, fmt.Errorf("client certificate is required unless AllowInsecure is set")
	}
	clientAuth := tls.RequireAndVerifyClientCert
	if cfg.AllowInsecure {
		cfg.Logger.Warn("allow_insecure_enabled", zap.String("transport", "tcp+tls"), zap.String("security", "tls_verification_disabled"), zap.String("usage", "development_only"))
		clientAuth = tls.NoClientCert
	}
	serverConf := &tls.Config{
		ClientCAs:    cfg.Roots,
		ClientAuth:   clientAuth,
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256},
		NextProtos:   []string{alpn},
	}
	clientConf := &tls.Config{
		RootCAs:            cfg.Roots,
		InsecureSkipVerify: cfg.AllowInsecure,
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		CipherSuites:       []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256},
		NextProtos:         []string{alpn},
	}
	if len(cfg.ServerCert.Certificate) > 0 {
		serverConf.Certificates = []tls.Certificate{cfg.ServerCert}
	}
	if len(cfg.ClientCert.Certificate) > 0 {
		clientConf.Certificates = []tls.Certificate{cfg.ClientCert}
	}
	return &Transport{serverConf: serverConf, clientConf: clientConf, logger: cfg.Logger}, nil
}

func init() {
	logger := zap.NewNop()
	if err := transport.MustRegister("tcp+tls", func(cfg transport.Config) (transport.Interface, error) {
		tr, err := New(cfg)
		if err != nil {
			return nil, err
		}
		if tr == nil {
			return nil, fmt.Errorf("tcp+tls: nil transport")
		}
		return tr, nil
	}); err != nil {
		logger.Error("register_failed", zap.String("transport", "tcp+tls"), zap.Error(err))
		rootcmd.SyncLogger(logger)
	}
}

func (t *Transport) Name() string { return "tcp+tls" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	role := "client"
	t.logger.Info("dial_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	dl, ok := ctx.Deadline()
	if !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultDialTimeout)
		defer cancel()
		dl, _ = ctx.Deadline()
	}
	d := &net.Dialer{Deadline: dl}
	tcpConn, err := d.DialContext(ctx, "tcp", address)
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
	tlsConf := t.clientConf.Clone()
	if tlsConf.ServerName == "" && !tlsConf.InsecureSkipVerify {
		host, _, _ := net.SplitHostPort(address)
		tlsConf.ServerName = host
	}
	if err := tcpConn.SetDeadline(dl); err != nil {
		tcpConn.Close()
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	conn := tls.Client(tcpConn, tlsConf)
	if err := conn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	state := conn.ConnectionState()
	if state.NegotiatedProtocol != alpn {
		err := fmt.Errorf("alpn mismatch: expected %q, got %q", alpn, state.NegotiatedProtocol)
		conn.Close()
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	if err := conn.SetDeadline(dl); err != nil {
		conn.Close()
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		t.logger.Error("dial_end", fields...)
		return nil, err
	}
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	t.logger.Info("dial_end", fields...)
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	role := "server"
	t.logger.Info("listen_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", address)
	if err == nil {
		ln = tls.NewListener(ln, t.serverConf)
		go func() {
			<-ctx.Done()
			if err := ln.Close(); err != nil {
				t.logger.Warn("listener_close_error", zap.Error(err))
			}
		}()
	}
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.Error(err),
	}
	if err != nil {
		t.logger.Error("listen_end", fields...)
	} else {
		fields = []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		}
		t.logger.Info("listen_end", fields...)
	}
	return ln, err
}

func (t *Transport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (peer common.Handshake, err error) {
	roleStr := role.String()
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
			fields = append(fields, transport.HandshakeFields(hs)...)
			t.logger.Info("negotiate_end", fields...)
		}
	}()

	if tlsConn, ok := conn.(*tls.Conn); ok {
		state := tlsConn.ConnectionState()
		if !state.HandshakeComplete {
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				return peer, err
			}
			state = tlsConn.ConnectionState()
		}
		negotiatedALPN := state.NegotiatedProtocol
		negotiatedVersion := logging.TLSVersionString(state.Version)
		if hs.ALPN != "" && negotiatedALPN != "" && hs.ALPN != negotiatedALPN {
			return peer, fmt.Errorf("alpn mismatch: %s", negotiatedALPN)
		}
		if hs.TLSVersion != "" && negotiatedVersion != "" && hs.TLSVersion != negotiatedVersion {
			return peer, fmt.Errorf("tls version mismatch: %s", negotiatedVersion)
		}
		if negotiatedALPN != "" {
			hs.ALPN = negotiatedALPN
		}
		if negotiatedVersion != "" {
			hs.TLSVersion = negotiatedVersion
		}
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
		hs = common.MergeHandshake(hs, peer)
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
		hs = common.MergeHandshake(hs, peer)
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
		return peer, fmt.Errorf("invalid role %v", role)
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
