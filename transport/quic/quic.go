package quic

import (
	"bufio"
	"context"
	"net"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

// Transport is a placeholder QUIC transport using plain TCP.
type Transport struct{}

// New returns a new Transport.
func New() transport.Interface { return &Transport{} }

func init() { transport.Register("quic", New) }

func (t *Transport) Name() string { return "quic" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	d := net.Dialer{}
	return d.DialContext(ctx, "tcp", address)
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	return net.Listen("tcp", address)
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
