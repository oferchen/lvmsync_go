package h2

import (
	"golang.org/x/net/http2"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func init() {
	transport.Register("h2", New)
}

// New returns no-op HTTP/2 transport implementations.
func New(cfg *config.Config) (transport.Sender, transport.Receiver, error) {
	_ = http2.ClientConn{}
	return transport.NopSender{}, transport.NopReceiver{}, nil
}
