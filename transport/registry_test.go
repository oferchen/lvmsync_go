package transport

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/common"
)

type orderStub struct{ name string }

func (s *orderStub) Name() string { return s.name }
func (s *orderStub) Dial(context.Context, string) (net.Conn, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *orderStub) Listen(context.Context, string) (net.Listener, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *orderStub) Negotiate(context.Context, net.Conn, Role, common.Handshake) (common.Handshake, error) {
	return common.Handshake{}, fmt.Errorf("not implemented")
}

func TestConcurrentRegister(t *testing.T) {
	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("test-%d", i)
			if err := Register(name, func(Config) (Interface, error) { return nil, nil }); err != nil {
				t.Errorf("register %s: %v", name, err)
			}
		}(i)
	}
	wg.Wait()
	logger := zap.NewNop()
	for i := 0; i < goroutines; i++ {
		name := fmt.Sprintf("test-%d", i)
		if _, err := Get(name, Config{Logger: logger}); err != nil {
			t.Errorf("get %s: %v", name, err)
		}
	}
	logger.Sync()
}

func TestDuplicateRegister(t *testing.T) {
	name := "dupe-test"
	if err := Register(name, func(Config) (Interface, error) { return nil, nil }); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := Register(name, func(Config) (Interface, error) { return nil, nil }); err == nil {
		t.Fatalf("expected duplicate registration error")
	}
}

func TestMustRegisterDuplicate(t *testing.T) {
	name := "must-dupe-test"
	if err := Register(name, func(Config) (Interface, error) { return nil, nil }); err != nil {
		t.Fatalf("first register: %v", err)
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
	}()
	MustRegister(name, func(Config) (Interface, error) { return nil, nil })
}

func TestGetOrdered(t *testing.T) {
	regMu.Lock()
	original := registry
	registry = map[string]Factory{}
	regMu.Unlock()
	defer func() {
		regMu.Lock()
		registry = original
		regMu.Unlock()
	}()

	Register("a", func(Config) (Interface, error) { return &orderStub{name: "a"}, nil })
	Register("b", func(Config) (Interface, error) { return &orderStub{name: "b"}, nil })

	trs, err := GetOrdered([]string{"b", "a"}, Config{Logger: zap.NewNop()})
	if err != nil {
		t.Fatalf("get ordered: %v", err)
	}
	if len(trs) != 2 || trs[0].Name() != "b" || trs[1].Name() != "a" {
		t.Fatalf("unexpected order: %v %v", trs[0].Name(), trs[1].Name())
	}
}
