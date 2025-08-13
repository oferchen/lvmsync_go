package h2

import (
	"bufio"
	"context"
	"fmt"
	"net"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

// Transport is a placeholder HTTP/2 transport using plain TCP.
type Transport struct{}

// New returns a new Transport.
func New() transport.Interface { return &Transport{} }

func init() {
	if err := transport.Register("h2", New); err != nil {
		panic(err)
	}
}

func (t *Transport) Name() string { return "h2" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	d := net.Dialer{}
	return d.DialContext(ctx, "tcp", address)
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	return net.Listen("tcp", address)
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
			return fmt.Errorf("endianness mismatch: %s", peer.Endianness)
		}
		return nil
	case transport.Server:
		peer, err := common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			return err
		}
		if peer.Endianness != "" && peer.Endianness != hs.Endianness {
			return fmt.Errorf("endianness mismatch: %s", peer.Endianness)
		}
		return common.WriteHandshake(conn, hs)
	default:
		return nil
	}
}
