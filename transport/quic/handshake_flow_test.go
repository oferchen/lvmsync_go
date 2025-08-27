package quic

import (
	"bufio"
	"context"
	"net"
	"testing"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
)

// TestClientHandshakeFailureWhenServerCloses verifies that the client-side
// handshake fails if the server reads the handshake and closes without
// responding.
func TestClientHandshakeFailureWhenServerCloses(t *testing.T) {
	tr := &Transport{logger: zap.NewNop()}
	cli, srv := net.Pipe()
	defer cli.Close()

	hs := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}

	done := make(chan struct{})
	go func() {
		buf := bufio.NewReader(srv)
		common.ReadHandshake(buf) // ignore result; simulate server read
		srv.Close()               // close without sending handshake
		close(done)
	}()

	if _, err := tr.Negotiate(context.Background(), cli, transport.Client, hs); err == nil {
		t.Fatalf("expected client negotiate error")
	}
	<-done
}

// TestServerHandshakeFailureWhenClientCloses verifies that the server-side
// handshake fails if the client disconnects before sending a handshake.
func TestServerHandshakeFailureWhenClientCloses(t *testing.T) {
	tr := &Transport{logger: zap.NewNop()}
	srv, cli := net.Pipe()
	defer srv.Close()

	hs := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}

	cli.Close() // client disconnects without sending handshake

	if _, err := tr.Negotiate(context.Background(), srv, transport.Server, hs); err == nil {
		t.Fatalf("expected server negotiate error")
	}
}
