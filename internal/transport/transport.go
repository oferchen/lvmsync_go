package transport

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"go.uber.org/zap"

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

// Factory constructs a sender/receiver pair using the provided configuration and logger.
type Factory func(cfg *config.Config, logger *zap.Logger) (Sender, Receiver, error)

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
// It logs lifecycle events for each attempt.
func Select(cfg *config.Config, order []string, logger *zap.Logger) (Sender, Receiver, string, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	for _, name := range order {
		logger.Info("initializing transport", zap.String("transport", name))
		f, ok := Get(name)
		if !ok {
			logger.Warn("transport not registered", zap.String("transport", name))
			continue
		}
		s, r, err := f(cfg, logger)
		if err == nil {
			logger.Info("transport initialized", zap.String("transport", name))
			return s, r, name, nil
		}
		logger.Warn("transport init failed", zap.String("transport", name), zap.Error(err))
	}
	logger.Error("no working transport", zap.Strings("transports", order))
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
