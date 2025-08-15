package tcp_tls

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

// New creates a Transport using provided TLS roots and client certificate.
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
		clientAuth = tls.RequireAnyClientCert
	}
	serverConf := &tls.Config{
		ClientCAs:  cfg.Roots,
		ClientAuth: clientAuth,
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{alpn},
	}
	clientConf := &tls.Config{
		RootCAs:            cfg.Roots,
		InsecureSkipVerify: cfg.AllowInsecure,
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{alpn},
	}
	if len(cert.Certificate) > 0 {
		serverConf.Certificates = []tls.Certificate{cert}
		clientConf.Certificates = []tls.Certificate{cert}
	}
	return &Transport{serverConf: serverConf, clientConf: clientConf, logger: cfg.Logger}, nil
}

func init() {
	if err := transport.Register("tcp+tls", New); err != nil {
		panic(err)
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
			ln.Close()
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

	if tlsConn, ok := conn.(*tls.Conn); ok {
		state := tlsConn.ConnectionState()
		negotiatedALPN := state.NegotiatedProtocol
		negotiatedVersion := transport.TLSVersionString(state.Version)
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
