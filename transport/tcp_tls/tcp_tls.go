package tcp_tls

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"time"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

// Transport implements TLS over TCP.
type Transport struct {
	serverConf *tls.Config
	clientConf *tls.Config
}

// New creates a Transport with a self-signed certificate for tests.
func New() transport.Interface {
	cert, _ := generateSelfSignedCert()
	return &Transport{
		serverConf: &tls.Config{Certificates: []tls.Certificate{cert}},
		clientConf: &tls.Config{InsecureSkipVerify: true},
	}
}

func init() { transport.Register("tcp+tls", New) }

func (t *Transport) Name() string { return "tcp+tls" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	d := net.Dialer{}
	return tls.DialWithDialer(&d, "tcp", address, t.clientConf)
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return tls.NewListener(ln, t.serverConf), nil
}

func (t *Transport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role) error {
	hs := common.Handshake{Version: common.ProtocolVersion}
	switch role {
	case transport.Client:
		if err := common.WriteHandshake(conn, hs); err != nil {
			return err
		}
		_, err := common.ReadHandshake(bufio.NewReader(conn))
		return err
	case transport.Server:
		if _, err := common.ReadHandshake(bufio.NewReader(conn)); err != nil {
			return err
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
