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

// New creates a Transport with a self-signed certificate for tests.
func New(logger *zap.Logger) transport.Interface {
	cert, _ := generateSelfSignedCert()
	return &Transport{
		serverConf: &tls.Config{Certificates: []tls.Certificate{cert}},
		clientConf: &tls.Config{InsecureSkipVerify: true},
		logger:     logger,
	}
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

func (t *Transport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role) error {
	hs := common.Handshake{Version: common.ProtocolVersion, Endianness: common.NativeEndianness()}
	switch role {
	case transport.Client:
		if err := common.WriteHandshake(conn, hs); err != nil {
			return err
		}
		peer, err := common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			return err
		}
		if peer.Endianness != "" && peer.Endianness != hs.Endianness {
			t.logger.Warn("endianness_mismatch", zap.String("peer_endianness", peer.Endianness), zap.String("local_endianness", hs.Endianness))
			return fmt.Errorf("endianness mismatch: %s", peer.Endianness)
		}
		return nil
	case transport.Server:
		peer, err := common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			return err
		}
		if peer.Endianness != "" && peer.Endianness != hs.Endianness {
			t.logger.Warn("endianness_mismatch", zap.String("peer_endianness", peer.Endianness), zap.String("local_endianness", hs.Endianness))
			return fmt.Errorf("endianness mismatch: %s", peer.Endianness)
		}
		return common.WriteHandshake(conn, hs)
	default:
		return nil
	}
}

func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return cert, nil
}
