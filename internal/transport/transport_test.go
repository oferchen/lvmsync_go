package transport

import (
	"errors"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

func TestRegistryLookup(t *testing.T) {
	factory := func(*config.Config, *zap.Logger) (Sender, Receiver, error) { return NopSender{}, NopReceiver{}, nil }
	Register("test", factory)
	got, ok := Get("test")
	if !ok {
		t.Fatalf("factory not found")
	}
	if got == nil {
		t.Fatalf("got nil factory")
	}
}

func TestSelectOrder(t *testing.T) {
	fail := func(*config.Config, *zap.Logger) (Sender, Receiver, error) { return nil, nil, errors.New("fail") }
	ok := func(*config.Config, *zap.Logger) (Sender, Receiver, error) { return NopSender{}, NopReceiver{}, nil }

	tests := []struct {
		name    string
		order   []string
		want    string
		wantErr bool
	}{
		{
			name:  "fallback",
			order: []string{"fail", "ok"},
			want:  "ok",
		},
		{
			name:    "allfail",
			order:   []string{"fail"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Register("fail", fail)
			Register("ok", ok)
			_, _, name, err := Select(&config.Config{}, tt.order, zap.NewNop())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("select returned error: %v", err)
			}
			if name != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, name)
			}
		})
	}
}
