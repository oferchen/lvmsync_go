package transfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
)

func TestVerifyDestinationNilContext(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	cfg := &config.Config{}
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, []byte("data"), 0600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	//lint:ignore SA1012 testing nil context handling
	if err := tr.verifyDestination(nil, cfg, dest); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestVerifyDestinationCanceledContext(t *testing.T) {
	info := device.NewInfoWithDeps(
		func(ctx context.Context, _ string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
		func(context.Context, string) (string, error) { return "", errors.New("no lvm") },
		func(context.Context, string) (bool, error) { return false, nil },
		nil,
	)
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, info)
	cfg := &config.Config{DeviceUUID: "id"}
	dest := filepath.Join(t.TempDir(), "dest")
	if err := os.WriteFile(dest, nil, 0600); err != nil {
		t.Fatalf("write dest: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { _, _, _, err := tr.verifyDestination(ctx, cfg, dest); errCh <- err }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("verifyDestination did not respect context cancellation")
	}
}
