package quic

import (
	"go.uber.org/zap"

	quic "github.com/quic-go/quic-go"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func init() {
	transport.Register("quic", New)
}

// New returns no-op QUIC transport implementations.
func New(cfg *config.Config, logger *zap.Logger) (transport.Sender, transport.Receiver, error) {
	_ = logger
	var _ quic.VersionNumber
	return transport.NopSender{}, transport.NopReceiver{}, nil
}
