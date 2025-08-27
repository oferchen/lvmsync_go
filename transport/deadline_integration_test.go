//go:build integration

package transport_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
	"github.com/oferchen/lvmsync_go/transport/testutil"

	_ "github.com/oferchen/lvmsync_go/transport/h2"
	_ "github.com/oferchen/lvmsync_go/transport/quic"
	_ "github.com/oferchen/lvmsync_go/transport/ssh"
	_ "github.com/oferchen/lvmsync_go/transport/tcp_tls"
)

func TestHandshakeDeadline(t *testing.T) {
	names := []string{"quic", "h2", "tcp+tls", "ssh"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tr := testutil.NewTransport(t, name)
			hs := common.Handshake{ALPN: "lvmsync", TLSVersion: "1.3", BlockSize: 4096}
			if name == "h2" {
				hs.ALPN = "h2"
			}
			timeout := 100 * time.Millisecond
			delay := 200 * time.Millisecond
			_, cliRes := testutil.RunNegotiationWithDelay(t, tr, hs, hs, delay, timeout)
			if cliRes.Err == nil || (!errors.Is(cliRes.Err, context.DeadlineExceeded) &&
				!strings.Contains(cliRes.Err.Error(), "deadline") &&
				!strings.Contains(cliRes.Err.Error(), "timeout")) {
				t.Fatalf("client error = %v, want deadline exceeded", cliRes.Err)
			}
		})
	}
}

func TestDialWithFallbackOrder(t *testing.T) {
	cert, pool := testutil.GenerateSelfSignedCert(t)
	core, obs := observer.New(zap.InfoLevel)
	cfg := transport.Config{
		Logger:        zap.New(core),
		ClientCert:    cert,
		ServerCert:    cert,
		Roots:         pool,
		SSHUser:       "user",
		SSHPassword:   "pass",
		AllowInsecure: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	names := []string{"quic", "h2", "tcp+tls", "ssh"}
	if _, _, err := transport.DialWithFallback(ctx, "127.0.0.1:9", names, cfg); err == nil {
		t.Fatalf("expected error")
	}
	var seq []string
	for _, entry := range obs.All() {
		if entry.Message == "dial_attempt" {
			if n, ok := entry.ContextMap()["transport"].(string); ok {
				seq = append(seq, n)
			}
		}
	}
	if !reflect.DeepEqual(seq, names) {
		t.Fatalf("attempt sequence %v, want %v", seq, names)
	}
}
