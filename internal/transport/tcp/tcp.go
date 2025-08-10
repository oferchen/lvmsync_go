package tcp

import (
	"crypto/tls"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func init() {
	transport.Register("tcp+tls", New)
}

// New returns no-op TCP+TLS transport implementations.
func New(cfg *config.Config) (transport.Sender, transport.Receiver, error) {
	_ = tls.Config{}
	return transport.NopSender{}, transport.NopReceiver{}, nil
}
