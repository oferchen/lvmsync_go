package transport

import (
	"context"
	"fmt"
	"net"
	"sync"

	"go.uber.org/zap"
)

// Factory creates a transport implementation.
type Factory func(Config) (Interface, error)

var (
	registry = map[string]Factory{}
	regMu    sync.RWMutex
)

// Register adds a transport factory to the registry.
func Register(name string, f Factory) error {
	regMu.Lock()
	defer regMu.Unlock()
	if _, exists := registry[name]; exists {
		return fmt.Errorf("transport %q already registered", name)
	}
	registry[name] = f
	return nil
}

// MustRegister registers a transport factory and panics on duplicate names.
// Deprecated: callers should prefer Register and handle the error.
func MustRegister(name string, f Factory) {
	if err := Register(name, f); err != nil {
		panic(fmt.Sprintf("register_transport %q: %v", name, err))
	}
}

// Get returns a transport from the registry by name.
func Get(name string, cfg Config) (Interface, error) {
	regMu.RLock()
	defer regMu.RUnlock()
	if f, ok := registry[name]; ok {
		return f(cfg)
	}
	return nil, fmt.Errorf("transport %q not registered", name)
}

// GetOrdered returns transports in the provided order using cfg for construction.
// An error is returned if any named transport is not registered.
func GetOrdered(names []string, cfg Config) ([]Interface, error) {
	trs := make([]Interface, 0, len(names))
	for _, n := range names {
		tr, err := Get(n, cfg)
		if err != nil {
			return nil, err
		}
		trs = append(trs, tr)
	}
	return trs, nil
}

// DialWithFallback tries transports in order until one successfully dials.
//
// Each attempt is logged at info level with a corresponding success or failure
// message. If all transports fail to dial, an error is returned.
func DialWithFallback(ctx context.Context, address string, names []string, cfg Config) (Interface, net.Conn, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	for _, name := range names {
		logger.Info("dial_attempt", zap.String("transport", name))
		tr, err := Get(name, cfg)
		if err != nil {
			logger.Warn("get_failed", zap.String("transport", name), zap.Error(err))
			continue
		}
		conn, err := tr.Dial(ctx, address)
		if err != nil {
			logger.Warn("dial_failed", zap.String("transport", name), zap.Error(err))
			continue
		}
		logger.Info("dial_success", zap.String("transport", name))
		return tr, conn, nil
	}
	return nil, nil, fmt.Errorf("all transports failed")
}
