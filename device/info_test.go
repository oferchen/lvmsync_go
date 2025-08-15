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

func TestGetUUIDStub(t *testing.T) {
	prev := SetUUIDFunc(func(ctx context.Context, path string) (string, error) {
		return "stub-uuid", nil
	})
	defer SetUUIDFunc(prev)

	got, err := GetUUID(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "stub-uuid" {
		t.Fatalf("expected stub-uuid, got %q", got)
	}
}

func TestIsMountedRW(t *testing.T) {
	tests := []struct {
		name string
		val  bool
	}{
		{"mounted", true},
		{"unmounted", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := SetMountFunc(func(path string) (bool, error) { return tt.val, nil })
			defer SetMountFunc(prev)

			got, err := IsMountedRW("/dev/sda")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.val {
				t.Fatalf("expected %v, got %v", tt.val, got)
			}
		})
	}
}
