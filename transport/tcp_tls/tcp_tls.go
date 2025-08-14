package tcp_tls

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

	"lvmsync_go/common"
	"lvmsync_go/transport"
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
	serverConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    cfg.Roots,
		ClientAuth:   clientAuth,
		MinVersion:   tls.VersionTLS13,
	}
	clientConf := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            cfg.Roots,
		InsecureSkipVerify: cfg.AllowInsecure,
		MinVersion:         tls.VersionTLS13,
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
	d := net.Dialer{}
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
