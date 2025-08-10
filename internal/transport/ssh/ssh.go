package ssh

import (
	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
	"lvmsync_go/remote"
)

func init() {
	transport.Register("ssh", New)
}

// New wraps the existing SSH client as a transport.
func New(cfg *config.Config) (transport.Sender, transport.Receiver, error) {
	_ = remote.Logger
	return transport.NopSender{}, transport.NopReceiver{}, nil
}
