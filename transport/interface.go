package transport

import (
	"context"
	"net"

	"lvmsync_go/common"
)

// Role identifies which side of the connection is performing negotiation.
type Role int

const (
	Client Role = iota
	Server
)

// String returns the string representation of the Role.
func (r Role) String() string {
	switch r {
	case Client:
		return "client"
	case Server:
		return "server"
	default:
		return ""
	}
}

// Interface defines the minimal methods required for transports.
//
// Implementations must establish a connection via Dial or Listen and then
// perform a protocol negotiation using Negotiate. Dial should connect to the
// remote address, while Listen should create a listener ready to accept
// connections. Negotiate exchanges the LVMSync handshake covering endianness,
// block size, deduplication mode, CDC parameters, compression, digest
// algorithms, resume tokens, maximum in-flight hints, and O_DIRECT capability
// before the connection is ready for use.
type Interface interface {
	Name() string
	Dial(ctx context.Context, address string) (net.Conn, error)
	Listen(ctx context.Context, address string) (net.Listener, error)
	Negotiate(ctx context.Context, conn net.Conn, role Role, hs common.Handshake) (common.Handshake, error)
}
