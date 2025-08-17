package quic

import (
	"context"
	"net"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func TestNewRequiresTLS(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop()}); err == nil {
		t.Fatalf("expected error when tls roots missing")
	}
}

func TestNegotiateWithPipe(t *testing.T) {
	tr := &Transport{logger: zap.NewNop()}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	hs := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() {
		_, err := tr.Negotiate(ctx, c1, transport.Client, hs)
		errCh <- err
	}()
	if _, err := tr.Negotiate(ctx, c2, transport.Server, hs); err != nil {
		t.Fatalf("server negotiate: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
}
