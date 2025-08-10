package transport

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"lvmsync_go/config"
)

// Sender pushes data to a remote peer.
type Sender interface {
	Send(ctx context.Context, r io.Reader) error
}

// Receiver accepts data from a remote peer.
type Receiver interface {
	Receive(ctx context.Context, w io.Writer) error
}

// Factory constructs a sender/receiver pair using the provided configuration.
type Factory func(cfg *config.Config) (Sender, Receiver, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

// Register adds a transport factory to the registry.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[strings.ToLower(name)] = f
}

// Get retrieves a transport factory by name.
func Get(name string) (Factory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := registry[strings.ToLower(name)]
	return f, ok
}

// Select returns the first transport in order that initializes successfully.
func Select(cfg *config.Config, order []string) (Sender, Receiver, string, error) {
	for _, name := range order {
		f, ok := Get(name)
		if !ok {
			continue
		}
		s, r, err := f(cfg)
		if err == nil {
			return s, r, name, nil
		}
	}
	return nil, nil, "", fmt.Errorf("no working transport")
}

// NopSender is a no-op sender implementation used by placeholder transports.
type NopSender struct{}

// Send implements the Sender interface.
func (NopSender) Send(ctx context.Context, r io.Reader) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if r == nil {
		return nil
	}
	_, err := io.Copy(io.Discard, r)
	return err
}

// NopReceiver is a no-op receiver implementation used by placeholder transports.
type NopReceiver struct{}

// Receive implements the Receiver interface.
func (NopReceiver) Receive(ctx context.Context, w io.Writer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if w == nil {
		return nil
	}
	_, err := w.Write(nil)
	return err
}
