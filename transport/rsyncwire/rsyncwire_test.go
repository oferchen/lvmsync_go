package rsyncwire

import (
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/transport"
)

func TestRequiresAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop()}); err == nil {
		t.Fatalf("expected error when AllowInsecure is false")
	}
}
