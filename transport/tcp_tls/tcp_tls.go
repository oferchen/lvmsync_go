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
	cert := cfg.ClientCert
	if len(cert.Certificate) == 0 {
		var err error
		cert, err = generateSelfSignedCert()
		if err != nil {
			return nil, err
		}
	}
	serverConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    cfg.Roots,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	clientConf := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            cfg.Roots,
		InsecureSkipVerify: cfg.Roots == nil,
		MinVersion:         tls.VersionTLS13,
	}
	return &Transport{serverConf: serverConf, clientConf: clientConf}, nil
}

func init() {
	if err := transport.Register("tcp+tls", New); err != nil {
		panic(err)
	}
}

func (t *Transport) Name() string { return "tcp+tls" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	t.logger.Info("dial", zap.String("address", address))
	d := net.Dialer{}
	return tls.DialWithDialer(&d, "tcp", address, t.clientConf)
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	t.logger.Info("listen", zap.String("address", address))
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return tls.NewListener(ln, t.serverConf), nil
}

func (t *Transport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (common.Handshake, error) {
	hs.Version = common.ProtocolVersion
	if hs.Endianness == "" {
		hs.Endianness = common.NativeEndianness()
	}
	switch role {
	case transport.Client:
		if err := common.WriteHandshake(conn, hs); err != nil {
			return common.Handshake{}, err
		}
		peer, err := common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			return common.Handshake{}, err
		}
		if peer.Endianness != "" && peer.Endianness != hs.Endianness {
			return peer, fmt.Errorf("endianness mismatch: %s", peer.Endianness)
		}
		return peer, nil
	case transport.Server:
		peer, err := common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			return common.Handshake{}, err
		}
		if peer.Endianness != "" && peer.Endianness != hs.Endianness {
			return peer, fmt.Errorf("endianness mismatch: %s", peer.Endianness)
		}
		if err := common.WriteHandshake(conn, hs); err != nil {
			return peer, err
		}
		return peer, nil
	default:
		return common.Handshake{}, nil
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
