package transport

import (
	"fmt"
	"sync"
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
