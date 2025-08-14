package ssh

import (
	"bufio"
	"context"
	"fmt"
	"net"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

// Transport implements the transport.Interface over plain TCP for now.
type Transport struct {
	cfg transport.Config
}

// New returns a new Transport.
func New(cfg transport.Config) (transport.Interface, error) {
	return &Transport{cfg: cfg}, nil
}

func init() {
	if err := transport.Register("ssh", New); err != nil {
		panic(err)
	}
}

func (t *Transport) Name() string { return "ssh" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	t.logger.Info("dial", zap.String("address", address))
	d := net.Dialer{}
	return d.DialContext(ctx, "tcp", address)
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	t.logger.Info("listen", zap.String("address", address))
	return net.Listen("tcp", address)
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
