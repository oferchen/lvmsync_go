package transport

import (
	"context"
	"net"
)

// Role identifies which side of the connection is performing negotiation.
type Role int

const (
	Client Role = iota
	Server
)

// Interface defines the minimal methods required for transports.
//
// Implementations must establish a connection via Dial or Listen and then
// perform a protocol negotiation using Negotiate. Dial should connect to the
// remote address, while Listen should create a listener ready to accept
// connections. Negotiate exchanges the protocol handshake and returns when the
// connection is ready for use.
type Interface interface {
	Name() string
	Dial(ctx context.Context, address string) (net.Conn, error)
	Listen(ctx context.Context, address string) (net.Listener, error)
	Negotiate(ctx context.Context, conn net.Conn, role Role) error
}
