package h2

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

// Transport is a placeholder HTTP/2 transport using plain TCP.
type Transport struct {
	cfg    transport.Config
	logger *zap.Logger
}

// New returns a new Transport.
func New(cfg transport.Config) (transport.Interface, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	return &Transport{cfg: cfg, logger: cfg.Logger}, nil
}

func init() {
	if err := transport.Register("h2", New); err != nil {
		panic(err)
	}
}

func (t *Transport) Name() string { return "h2" }

func (t *Transport) Dial(ctx context.Context, address string) (net.Conn, error) {
	role := "client"
	t.logger.Info("dial_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
		zap.String("error", ""),
	)
	start := time.Now()
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", address)
	t.logger.Info("dial_end",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("error", errString(err)),
	)
	return conn, err
}

func (t *Transport) Listen(ctx context.Context, address string) (net.Listener, error) {
	role := "server"
	t.logger.Info("listen_start",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", 0),
		zap.String("error", ""),
	)
	start := time.Now()
	ln, err := net.Listen("tcp", address)
	t.logger.Info("listen_end",
		zap.String("address", address),
		zap.String("role", role),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("error", errString(err)),
	)
	return ln, err
}

func (t *Transport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (peer common.Handshake, err error) {
	roleStr := roleString(role)
	address := conn.RemoteAddr().String()
	t.logger.Info("negotiate_start",
		zap.String("address", address),
		zap.String("role", roleStr),
		zap.Int64("duration_ms", 0),
		zap.String("error", ""),
	)
	start := time.Now()
	defer func() {
		t.logger.Info("negotiate_end",
			zap.String("address", address),
			zap.String("role", roleStr),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.String("error", errString(err)),
		)
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
		if peer.Endianness != "" && peer.Endianness != hs.Endianness {
			return peer, fmt.Errorf("endianness mismatch: %s", peer.Endianness)
		}
		return peer, nil
	case transport.Server:
		peer, err = common.ReadHandshake(bufio.NewReader(conn))
		if err != nil {
			return peer, err
		}
		if peer.Endianness != "" && peer.Endianness != hs.Endianness {
			return peer, fmt.Errorf("endianness mismatch: %s", peer.Endianness)
		}
		if err = common.WriteHandshake(conn, hs); err != nil {
			return peer, err
		}
		return peer, nil
	default:
		return peer, nil
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
