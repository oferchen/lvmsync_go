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
type Transport struct{ logger *zap.Logger }

// New returns a new Transport.
func New(logger *zap.Logger) transport.Interface { return &Transport{logger: logger} }

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
