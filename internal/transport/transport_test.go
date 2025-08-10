package transport

import (
	"errors"
	"testing"

	"lvmsync_go/config"
)

func TestRegistryLookup(t *testing.T) {
	factory := func(*config.Config) (Sender, Receiver, error) { return NopSender{}, NopReceiver{}, nil }
	Register("test", factory)
	got, ok := Get("test")
	if !ok {
		t.Fatalf("factory not found")
	}
	if got == nil {
		t.Fatalf("got nil factory")
	}
}

func TestSelectFallback(t *testing.T) {
	fail := func(*config.Config) (Sender, Receiver, error) { return nil, nil, errors.New("fail") }
	ok := func(*config.Config) (Sender, Receiver, error) { return NopSender{}, NopReceiver{}, nil }
	Register("fail", fail)
	Register("ok", ok)
	_, _, name, err := Select(&config.Config{}, []string{"fail", "ok"})
	if err != nil {
		t.Fatalf("select returned error: %v", err)
	}
	if name != "ok" {
		t.Fatalf("expected ok, got %s", name)
	}
}
