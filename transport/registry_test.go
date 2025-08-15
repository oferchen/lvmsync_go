package transport

import (
	"fmt"
	"sync"
	"testing"

	"go.uber.org/zap"
)

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
