package transport

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentRegister(t *testing.T) {
	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("test-%d", i)
			if err := Register(name, func() Interface { return nil }); err != nil {
				t.Errorf("register %s: %v", name, err)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < goroutines; i++ {
		name := fmt.Sprintf("test-%d", i)
		if _, err := Get(name); err != nil {
			t.Errorf("get %s: %v", name, err)
		}
	}
}

func TestDuplicateRegister(t *testing.T) {
	name := "dupe-test"
	if err := Register(name, func() Interface { return nil }); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := Register(name, func() Interface { return nil }); err == nil {
		t.Fatalf("expected duplicate registration error")
	}
}
