package device

import (
	"context"
	"errors"
	"testing"
)

func TestGetUUIDCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GetUUID(ctx, "/dev/null"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}
