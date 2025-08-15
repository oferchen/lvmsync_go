package transport

import (
	"fmt"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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
	core, logs := observer.New(zapcore.FatalLevel)
	logger := zap.New(core, zap.WithFatalHook(zapcore.WriteThenPanic))
	zap.ReplaceGlobals(logger)
	defer func() {
		zap.ReplaceGlobals(zap.NewNop())
		logger.Sync()
	}()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
		if logs.Len() != 1 {
			t.Fatalf("expected 1 log entry, got %d", logs.Len())
		}
		entry := logs.All()[0]
		if entry.Level != zapcore.FatalLevel {
			t.Fatalf("expected fatal level, got %v", entry.Level)
		}
		if entry.Message != "register_transport" {
			t.Fatalf("unexpected message: %s", entry.Message)
		}
	}()
	MustRegister(name, func(Config) (Interface, error) { return nil, nil })
}
