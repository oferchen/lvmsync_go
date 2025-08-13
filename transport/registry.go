package transport

import "fmt"

// Factory creates a transport implementation.
type Factory func() Interface

var registry = map[string]Factory{}

// Register adds a transport factory to the registry.
func Register(name string, f Factory) {
	registry[name] = f
}

// Get returns a transport from the registry by name.
func Get(name string) (Interface, error) {
	if f, ok := registry[name]; ok {
		return f(), nil
	}
	return nil, fmt.Errorf("transport %q not registered", name)
}
