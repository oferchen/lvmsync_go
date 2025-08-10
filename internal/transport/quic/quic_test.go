package quic

import (
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func newPair(t *testing.T) (transport.Sender, transport.Receiver) {
	t.Helper()
	f, ok := transport.Get("quic")
	if !ok {
		t.Fatalf("quic transport not registered")
	}
	// Create receiver first to determine listening address.
	cfgR := &config.Config{QUICListen: "127.0.0.1:0"}
	_, rcv, err := f(cfgR)
	if err != nil {
		t.Fatalf("receiver factory error: %v", err)
	}
	qr, ok := rcv.(*quicReceiver)
	if !ok {
		t.Fatalf("unexpected receiver type")
	}
	cfgS := &config.Config{QUICConnect: qr.ln.Addr().String()}
	snd, _, err := f(cfgS)
	if err != nil {
		t.Fatalf("sender factory error: %v", err)
	}
	return snd, rcv
}

func TestQUICSendReceive(t *testing.T) {
	s, r := newPair(t)
	defer r.(*quicReceiver).Close()
	ctx := context.Background()
	data := []byte("hello quic")
	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := r.Receive(ctx, &buf); err != nil {
			t.Errorf("receive: %v", err)
		}
	}()
	if err := s.Send(ctx, bytes.NewReader(data)); err != nil {
		t.Fatalf("send: %v", err)
	}
	wg.Wait()
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("data mismatch")
	}
}

func TestQUICContextCancel(t *testing.T) {
	s, r := newPair(t)
	defer r.(*quicReceiver).Close()
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

func TestQUICSendErrorPropagation(t *testing.T) {
	s, r := newPair(t)
	defer r.(*quicReceiver).Close()
	rErr := errors.New("read fail")
	if err := s.Send(context.Background(), errReader{rErr}); !errors.Is(err, rErr) {
		t.Fatalf("send expected %v, got %v", rErr, err)
	}
}

func TestQUICIntegration(t *testing.T) {
	s, r := newPair(t)
	defer r.(*quicReceiver).Close()
	ctx := context.Background()
	data := []byte("integration test")
	var buf bytes.Buffer
	errCh := make(chan error, 1)
	go func() { errCh <- r.Receive(ctx, &buf) }()
	if err := s.Send(ctx, bytes.NewReader(data)); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("receive: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("data mismatch")
	}
}

func TestQUICRegistered(t *testing.T) {
	if _, ok := transport.Get("quic"); !ok {
		t.Fatalf("quic transport not registered")
	}
}

func TestQUICNew(t *testing.T) {
	if _, _, err := New(&config.Config{}, zap.NewNop()); err != nil {
		t.Fatalf("New: %v", err)
	}
}

