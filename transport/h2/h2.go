package h2

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/net/http2"

	"lvmsync_go/common"
	"lvmsync_go/internal/logging"
	"lvmsync_go/transport"
)

const (
	maxFrameSize       = 1 << 14
	defaultDialTimeout = 5 * time.Second
)

// Transport implements HTTP/2 over TLS1.3 with mutual authentication.
type Transport struct {
	clientConf    *tls.Config
	serverConf    *tls.Config
	logger        *zap.Logger
	clearDeadline func(net.Conn) error
}

// Conn wraps a single HTTP/2 stream to satisfy net.Conn.
type Conn struct {
	net.Conn
	fr       *http2.Framer
	streamID uint32
	tlsState tls.ConnectionState
	readMu   sync.Mutex
	writeMu  sync.Mutex
	readBuf  bytes.Buffer
	closed   bool
}

// listener adapts a TLS listener and performs HTTP/2 handshakes.
type listener struct {
	ln  net.Listener
	ctx context.Context
}

// New returns a new Transport. Logger is optional; a no-op logger is used when nil.
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
		cfg.Logger.Warn("allow_insecure_enabled", zap.String("transport", "h2"))
		clientAuth = tls.RequireAnyClientCert
	}
	clientConf := &tls.Config{
		RootCAs:            cfg.Roots,
		NextProtos:         []string{"h2"},
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		CipherSuites:       []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256},
		InsecureSkipVerify: cfg.AllowInsecure,
	}
	if len(cfg.ClientCert.Certificate) != 0 {
		clientConf.Certificates = []tls.Certificate{cfg.ClientCert}
	}
	serverConf := &tls.Config{
		ClientCAs:    cfg.Roots,
		ClientAuth:   clientAuth,
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256},
		NextProtos:   []string{"h2"},
	}
	if len(cfg.ServerCert.Certificate) != 0 {
		serverConf.Certificates = []tls.Certificate{cfg.ServerCert}
	}
	return &Transport{
		clientConf:    clientConf,
		serverConf:    serverConf,
		logger:        cfg.Logger,
		clearDeadline: func(conn net.Conn) error { return conn.SetDeadline(time.Time{}) },
	}, nil
}

func init() {
	if err := transport.Register("h2", New); err != nil {
		panic(err)
	}
}

func (t *Transport) Name() string { return "h2" }

func dialTLS(ctx context.Context, address string, conf *tls.Config, logger *zap.Logger) (*tls.Conn, error) {
	role := "client"
	logger.Info("tls_handshake_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	d := net.Dialer{}
	if dl, ok := ctx.Deadline(); ok {
		d.Deadline = dl
	} else {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultDialTimeout)
		defer cancel()
		if dl, ok := ctx.Deadline(); ok {
			d.Deadline = dl
		}
	}
	conn, err := tls.DialWithDialer(&d, "tcp", address, conf)
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		logger.Error("tls_handshake_end", fields...)
		return nil, err
	}
	fields = append(fields, zap.String("tls_version", logging.TLSVersionString(conn.ConnectionState().Version)))
	logger.Info("tls_handshake_end", fields...)
	return conn, nil
}

func performH2Handshake(ctx context.Context, conn *tls.Conn, logger *zap.Logger) (*http2.Framer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	role := "client"
	address := conn.RemoteAddr().String()
	logger.Info("h2_handshake_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	fr := http2.NewFramer(conn, conn)

	do := func(op func() error) error {
		if err := setDeadline(ctx, conn); err != nil {
			return err
		}
		defer clearDeadline(conn)
		if err := ctx.Err(); err != nil {
			return err
		}
		return op()
	}

	if err := do(func() error {
		_, err := conn.Write([]byte(http2.ClientPreface))
		return err
	}); err != nil {
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		logger.Error("h2_handshake_end", fields...)
		return nil, err
	}
	if err := do(func() error { return fr.WriteSettings() }); err != nil {
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		logger.Error("h2_handshake_end", fields...)
		return nil, err
	}
	var f http2.Frame
	if err := do(func() error {
		var err error
		f, err = fr.ReadFrame()
		return err
	}); err != nil {
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		logger.Error("h2_handshake_end", fields...)
		return nil, err
	} else if _, ok := f.(*http2.SettingsFrame); !ok {
		err := fmt.Errorf("expected settings frame")
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		logger.Error("h2_handshake_end", fields...)
		return nil, err
	}
	if err := do(func() error { return fr.WriteSettingsAck() }); err != nil {
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		logger.Error("h2_handshake_end", fields...)
		return nil, err
	}
	if err := do(func() error {
		var err error
		f, err = fr.ReadFrame()
		return err
	}); err != nil {
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		logger.Error("h2_handshake_end", fields...)
		return nil, err
	} else if sf, ok := f.(*http2.SettingsFrame); !ok || !sf.IsAck() {
		err := fmt.Errorf("expected settings ack")
		fields := []zap.Field{
			zap.String("address", address),
			zap.String("role", role),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.Error(err),
		}
		logger.Error("h2_handshake_end", fields...)
		return nil, err
	}
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	logger.Info("h2_handshake_end", fields...)
	return fr, nil
}

func logDialResult(ctx context.Context, logger *zap.Logger, address, role string, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		logger.Error("dial_end", fields...)
		return
	}
	logger.Info("dial_end", fields...)
}

// Dial establishes a TLS1.3 connection and performs an HTTP/2 handshake.
func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	role := "client"
	t.logger.Info("dial_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
	)
	start := time.Now()
	conn, err := dialTLS(ctx, address, t.clientConf, t.logger)
	if err != nil {
		logDialResult(ctx, t.logger, address, role, start, err)
		return nil, err
	}
	fr, err := performH2Handshake(ctx, conn, t.logger)
	if err != nil {
		conn.Close()
		logDialResult(ctx, t.logger, address, role, start, err)
		return nil, err
	}
	logDialResult(ctx, t.logger, address, role, start, nil)
	if err := t.clearDeadline(conn); err != nil {
		t.logger.Error("clear_deadline_failed", zap.Error(err))
		conn.Close()
		return nil, err
	}
	return &Conn{
		Conn:     conn,
		fr:       fr,
		streamID: 1,
		tlsState: conn.ConnectionState(),
	}, nil
}

// Listen starts a TLS listener that negotiates HTTP/2 connections.
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
	}
	t.logger.Info("listen_end", fields...)
	return &listener{ln: ln, ctx: ctx}, nil
}

func (l *listener) Accept() (net.Conn, error) {
	conn, err := l.ln.Accept()
	if err != nil {
		return nil, err
	}
	if dl, ok := l.ctx.Deadline(); ok {
		if err := conn.SetDeadline(dl); err != nil {
			conn.Close()
			return nil, err
		}
	}
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("expected tls.Conn")
	}
	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, err
	}
	fr := http2.NewFramer(tlsConn, tlsConn)
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(tlsConn, preface); err != nil {
		conn.Close()
		return nil, err
	}
	if string(preface) != http2.ClientPreface {
		conn.Close()
		return nil, fmt.Errorf("invalid preface")
	}
	if f, err := fr.ReadFrame(); err != nil {
		conn.Close()
		return nil, err
	} else if _, ok := f.(*http2.SettingsFrame); !ok {
		conn.Close()
		return nil, fmt.Errorf("expected settings frame")
	}
	if err := fr.WriteSettings(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := fr.WriteSettingsAck(); err != nil {
		conn.Close()
		return nil, err
	}
	if f, err := fr.ReadFrame(); err != nil {
		conn.Close()
		return nil, err
	} else if sf, ok := f.(*http2.SettingsFrame); !ok || !sf.IsAck() {
		conn.Close()
		return nil, fmt.Errorf("expected settings ack")
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return &Conn{
		Conn:     tlsConn,
		fr:       fr,
		streamID: 1,
		tlsState: tlsConn.ConnectionState(),
	}, nil
}

func (l *listener) Close() error   { return l.ln.Close() }
func (l *listener) Addr() net.Addr { return l.ln.Addr() }

// Negotiate exchanges LVMSync handshake messages over the HTTP/2 stream.
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

	if h2c, ok := conn.(*Conn); ok {
		negotiatedALPN := h2c.tlsState.NegotiatedProtocol
		negotiatedVersion := logging.TLSVersionString(h2c.tlsState.Version)
		if hs.ALPN != "" && negotiatedALPN != "" && hs.ALPN != negotiatedALPN {
			return peer, fmt.Errorf("alpn mismatch: %s", negotiatedALPN)
		}
		if hs.TLSVersion != "" && negotiatedVersion != "" && hs.TLSVersion != negotiatedVersion {
			return peer, fmt.Errorf("tls version mismatch: %s", negotiatedVersion)
		}
		hs.ALPN = negotiatedALPN
		hs.TLSVersion = negotiatedVersion
	} else if tlsConn, ok := conn.(*tls.Conn); ok {
		state := tlsConn.ConnectionState()
		negotiatedALPN := state.NegotiatedProtocol
		negotiatedVersion := logging.TLSVersionString(state.Version)
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

// Conn implements net.Conn using HTTP/2 DATA frames.
func (c *Conn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.readBuf.Len() == 0 && c.closed {
		return 0, io.EOF
	}
	for c.readBuf.Len() == 0 {
		f, err := c.fr.ReadFrame()
		if err != nil {
			return 0, err
		}
		df, ok := f.(*http2.DataFrame)
		if !ok || df.Header().StreamID != c.streamID {
			continue
		}
		c.readBuf.Write(df.Data())
		if df.StreamEnded() {
			c.closed = true
			break
		}
	}
	return c.readBuf.Read(p)
}

func (c *Conn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	written := 0
	for len(p) > 0 {
		n := len(p)
		if n > maxFrameSize {
			n = maxFrameSize
		}
		if err := c.fr.WriteData(c.streamID, false, p[:n]); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}

func (c *Conn) Close() error {
	c.writeMu.Lock()
	err1 := c.fr.WriteData(c.streamID, true, nil)
	c.writeMu.Unlock()
	err2 := c.Conn.Close()
	return multierr.Append(err1, err2)
}

func (c *Conn) SetDeadline(t time.Time) error      { return c.Conn.SetDeadline(t) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.Conn.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.Conn.SetWriteDeadline(t) }
