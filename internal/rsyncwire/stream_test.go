package rsyncwire

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStreamRecvWithinLimit(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	const max = 4
	sender := NewStream(c1, max)
	recv := NewStream(c2, max)

	payload := []byte("test")
	go func() {
		if err := sender.Send(context.Background(), payload); err != nil {
			t.Errorf("Send: %v", err)
		}
	}()

	frame, err := recv.Recv(context.Background())
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if string(frame) != string(payload) {
		t.Fatalf("unexpected payload %q", string(frame))
	}
}

func TestStreamRecvTooLarge(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	const max = 4
	recv := NewStream(c2, max)

	payload := []byte("hello")
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(hdr[4:8], crc32.Checksum(payload, crcTable))
	go func() {
		c1.Write(hdr[:])
	}()

	if _, err := recv.Recv(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected size limit error, got %v", err)
	}
}

func TestStreamSendTooLarge(t *testing.T) {
	var buf bytes.Buffer
	const max = 4
	s := NewStream(&buf, max)
	payload := []byte("hello")
	if err := s.Send(context.Background(), payload); err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected size limit error, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no data written, got %d bytes", buf.Len())
	}
}

type shortReadWriter struct {
	bytes.Buffer
	max   int
	calls int
}

func (s *shortReadWriter) Write(p []byte) (int, error) {
	s.calls++
	if len(p) > s.max {
		p = p[:s.max]
	}
	return s.Buffer.Write(p)
}

// TestStreamSendShortWrites ensures Send retries until all bytes are written.
func TestStreamSendShortWrites(t *testing.T) {
	const limit = 3
	srw := &shortReadWriter{max: limit}
	s := NewStream(srw, 1<<20)
	payload := []byte("hello world")
	if err := s.Send(context.Background(), payload); err != nil {
		t.Fatalf("Send: %v", err)
	}
	expectedLen := 8 + len(payload)
	if srw.Len() != expectedLen {
		t.Fatalf("wrote %d bytes, want %d", srw.Len(), expectedLen)
	}
	data := srw.Bytes()
	n := binary.BigEndian.Uint32(data[0:4])
	if int(n) != len(payload) {
		t.Fatalf("header length %d != payload length %d", n, len(payload))
	}
	crc := binary.BigEndian.Uint32(data[4:8])
	if crc != crc32.Checksum(payload, crcTable) {
		t.Fatalf("crc mismatch")
	}
	// Expect multiple writes due to short writer.
	expectedCalls := (expectedLen + limit - 1) / limit
	if srw.calls != expectedCalls {
		t.Fatalf("expected %d writes, got %d", expectedCalls, srw.calls)
	}
}

func TestStreamRecvTimeout(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	recv := NewStream(c2, 1<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if _, err := recv.Recv(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestStreamSendTimeout(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	sender := NewStream(c1, 1<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		payload := bytes.Repeat([]byte("a"), 1<<20)
		errCh <- sender.Send(ctx, payload)
	}()
	if err := <-errCh; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	_ = c2.Close()
}

// TestStreamNoGoroutineLeak ensures Send and Recv do not spawn lingering goroutines
// when contexts are not cancelled.
func TestStreamNoGoroutineLeak(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	sender := NewStream(c1, 1<<20)
	receiver := NewStream(c2, 1<<20)

	payload := []byte("data")
	start := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		sendCtx, _ := context.WithCancel(context.Background())
		recvCtx, _ := context.WithCancel(context.Background())

		errCh := make(chan error, 1)
		go func() { errCh <- sender.Send(sendCtx, payload) }()
		if _, err := receiver.Recv(recvCtx); err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if err := <-errCh; err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	// Allow any goroutines to exit and force GC.
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()
	if diff := after - start; diff > 5 {
		t.Fatalf("goroutines leaked: before=%d after=%d", start, after)
	}
}

func BenchmarkStreamSend(b *testing.B) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	sender := NewStream(c1, 1<<20)
	go func() {
		buf := make([]byte, 1<<20)
		for {
			if _, err := c2.Read(buf); err != nil {
				return
			}
		}
	}()
	payload := []byte("benchmark")
	for i := 0; i < b.N; i++ {
		if err := sender.Send(context.Background(), payload); err != nil {
			b.Fatalf("Send: %v", err)
		}
	}
}
