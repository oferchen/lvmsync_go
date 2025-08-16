package common

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestOpenWithContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := OpenWithContext(ctx, "nonexistent")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestOpenWithContextSuccess(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmp.Close()
	f, err := OpenWithContext(context.Background(), tmp.Name())
	if err != nil {
		t.Fatalf("OpenWithContext: %v", err)
	}
	f.Close()
}
