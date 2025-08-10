package quic

import (
	quic "github.com/quic-go/quic-go"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func init() {
	transport.Register("quic", New)
}

// New returns no-op QUIC transport implementations.
func New(cfg *config.Config) (transport.Sender, transport.Receiver, error) {
	var _ quic.VersionNumber
	return transport.NopSender{}, transport.NopReceiver{}, nil
}
