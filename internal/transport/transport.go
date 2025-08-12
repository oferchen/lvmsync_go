package transport

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

// Transport defines the common data-plane operations used by LVMSync
// transports. Implementations may stream chunks to a remote peer and
// receive bitmap responses.
type Transport interface {
	Open(ctx context.Context) error
	SendChunk(index uint64, flags uint16, hash []byte, payload []byte) error
	RecvBitmap(ctx context.Context) ([]byte, error)
	Flush() error
	Close() error
}

// Factory constructs a transport using the provided configuration and logger.
type Factory func(cfg *config.Config, logger *zap.Logger) (Transport, error)

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
func Select(cfg *config.Config, order []string, logger *zap.Logger) (Transport, string, error) {
	for _, name := range order {
		f, ok := Get(name)
		if !ok {
			continue
		}
		t, err := f(cfg, logger)
		if err == nil {
			return t, name, nil
		}
	}
	return nil, "", fmt.Errorf("no working transport")
}

// NopTransport is a no-op implementation used by placeholder transports and tests.
type NopTransport struct{}

// Open implements Transport.
func (NopTransport) Open(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// SendChunk implements Transport.
func (NopTransport) SendChunk(index uint64, flags uint16, hash []byte, payload []byte) error {
	_ = index
	_ = flags
	_ = hash
	_ = payload
	return nil
}

// RecvBitmap implements Transport.
func (NopTransport) RecvBitmap(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, nil
	}
}

// Flush implements Transport.
func (NopTransport) Flush() error { return nil }

// Close implements Transport.
func (NopTransport) Close() error { return nil }
