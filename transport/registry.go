package transport

import (
	"fmt"
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

// MustRegister registers a transport factory and logs a fatal error on duplicates.
func MustRegister(name string, f Factory) {
	if err := Register(name, f); err != nil {
		zap.L().Fatal("register_transport", zap.String("name", name), zap.Error(err))
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
