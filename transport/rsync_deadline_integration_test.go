//go:build integration

package transport_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
	"lvmsync_go/transport/testutil"

	_ "lvmsync_go/transport/rsyncwire"
)

func TestRsyncHandshakeDeadline(t *testing.T) {
	cfg := transport.Config{Logger: zap.NewNop(), AllowInsecure: true}
	tr, err := transport.Get("rsync", cfg)
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}
	hs := common.Handshake{ALPN: "lvmsync", TLSVersion: "1.3", BlockSize: 4096}
	timeout := 100 * time.Millisecond
	delay := 200 * time.Millisecond
	_, cliRes := testutil.RunNegotiationWithDelay(t, tr, hs, hs, delay, timeout)
	if cliRes.Err == nil || (!errors.Is(cliRes.Err, context.DeadlineExceeded) &&
		!strings.Contains(cliRes.Err.Error(), "deadline") &&
		!strings.Contains(cliRes.Err.Error(), "timeout")) {
		t.Fatalf("client error = %v, want deadline exceeded", cliRes.Err)
	}
}
