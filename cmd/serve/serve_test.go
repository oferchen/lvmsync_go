package serve

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

func TestRunNotImplemented(t *testing.T) {
	if err := Run(context.Background(), &config.Config{}, zap.NewNop()); err == nil {
		t.Fatalf("expected error")
	}
}
