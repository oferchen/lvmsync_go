package h2

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func getH2(t *testing.T) (transport.Sender, transport.Receiver) {
	t.Helper()
	f, ok := transport.Get("h2")
	if !ok {
    		t.Fatalf("h2 can't get transport!")
	}
}

func TestH2Registered(t *testing.T) {
	if _, ok := transport.Get("h2"); !ok {
		t.Fatalf("h2 transport not registered")
	}
}

func TestH2Transfer(t *testing.T) {
	f, _ := transport.Get("h2")
	cfg := &config.Config{H2Port: 0}
	sender, receiver, err := f(cfg)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	return s, r
}


func TestH2ContextCancel(t *testing.T) {
	f, _ := transport.Get("h2")
	cfg := &config.Config{H2Port: 0}
	sender, receiver, err := f(cfg)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	rctx, rcancel := context.WithTimeout(context.Background(), time.Second)
	errCh := make(chan error, 1)
	go func() { errCh <- receiver.Receive(rctx, &buf) }()
	if err := sender.Send(ctx, bytes.NewReader([]byte("cancel"))); err == nil {
		t.Fatalf("expected error on canceled context")
	}
	rcancel()
	<-errCh
}

func TestH2ContextCancel(t *testing.T) {
	s, r := getH2(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Send(ctx, bytes.NewReader(nil)); !errors.Is(err, context.Canceled) {
		t.Fatalf("send expected context.Canceled, got %v", err)
	}
	if err := r.Receive(ctx, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("receive expected context.Canceled, got %v", err)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

type errWriter struct{ err error }

func (e errWriter) Write([]byte) (int, error) { return 0, e.err }

func TestH2ErrorPropagation(t *testing.T) {
	s, r := getH2(t)
	rErr := errors.New("read fail")
	if err := s.Send(context.Background(), errReader{rErr}); !errors.Is(err, rErr) {
		t.Fatalf("send expected %v, got %v", rErr, err)
	}
	wErr := errors.New("write fail")
	if err := r.Receive(context.Background(), errWriter{wErr}); !errors.Is(err, wErr) {
		t.Fatalf("receive expected %v, got %v", wErr, err)
	}
}

func TestH2Integration(t *testing.T) {
	s, r := getH2(t)
	ctx := context.Background()
	pr, pw := io.Pipe()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := s.Send(ctx, pr); err != nil {
			t.Errorf("send: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		defer pw.Close()
		if err := r.Receive(ctx, pw); err != nil {
			t.Errorf("receive: %v", err)
		}
	}()
	wg.Wait()
}
