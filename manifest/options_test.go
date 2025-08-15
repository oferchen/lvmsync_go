package manifest

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"lvmsync_go/device"
)

func TestApplyOptions(t *testing.T) {
	var detectCalled, hookCalled bool
	opts := applyOptions([]IndexOption{
		WithDetectDevice(func(context.Context, string, *zap.Logger) (device.Device, error) {
			detectCalled = true
			return nil, nil
		}),
		WithCloseHook(func() error {
			hookCalled = true
			return nil
		}),
	})
	if _, err := opts.detectDevice(context.Background(), "", zap.NewNop()); err != nil {
		t.Fatalf("detectDevice error: %v", err)
	}
	if err := opts.closeHook(); err != nil {
		t.Fatalf("closeHook error: %v", err)
	}
	if !detectCalled {
		t.Fatalf("detectDevice not called")
	}
	if !hookCalled {
		t.Fatalf("closeHook not called")
	}
}
