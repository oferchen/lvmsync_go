package transport_test

import (
	"sync"
	"testing"
	_ "unsafe"

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
	_ "github.com/oferchen/lvmsync_go/transport/h2"
	_ "github.com/oferchen/lvmsync_go/transport/quic"
	_ "github.com/oferchen/lvmsync_go/transport/ssh"
	_ "github.com/oferchen/lvmsync_go/transport/tcp_tls"
	"github.com/oferchen/lvmsync_go/transport/testutil"
)

//go:linkname registry github.com/oferchen/lvmsync_go/transport.registry
var registry map[string]transport.Factory

//go:linkname regMu github.com/oferchen/lvmsync_go/transport.regMu
var regMu sync.RWMutex

func TestHandshakeValidationMismatch(t *testing.T) {
	regMu.RLock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	regMu.RUnlock()
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			if name == "rsync" {
				t.Skip("rsync transport lacks ALPN/TLS validation")
			}
			tr := testutil.NewTransport(t, name)
			serverHS := common.Handshake{ALPN: "lvmsync", TLSVersion: "1.3"}
			if name == "h2" {
				serverHS.ALPN = "h2"
			}
			clientHS := serverHS
			clientHS.ALPN = "other"
			clientHS.TLSVersion = "1.2"
			srv, cli := testutil.RunNegotiation(t, tr, serverHS, clientHS)
			if srv.Err == nil || cli.Err == nil {
				t.Fatalf("expected negotiation error: server %v client %v", srv.Err, cli.Err)
			}
		})
	}
}
