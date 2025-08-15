package dump

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// readerBlockingOnContext waits for ctx cancellation before returning.
type readerBlockingOnContext struct {
	ctx context.Context
}

func (r *readerBlockingOnContext) Read(p []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

// readerWithRelease returns data only after release is closed.
type readerWithRelease struct {
	data    []byte
	release chan struct{}
}

func (r *readerWithRelease) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	<-r.release
	return n, io.EOF
}

func TestCopyPipeAsyncCanceledDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &readerBlockingOnContext{ctx: ctx}
	var dst bytes.Buffer
	errCh := CopyPipeAsync(ctx, &dst, r)
	time.AfterFunc(10*time.Millisecond, cancel)
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if dst.Len() != 0 {
		t.Fatalf("expected no bytes written, got %d", dst.Len())
	}
}

func TestCopyPipeAsyncCanceledDuringWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &readerWithRelease{data: []byte("hello"), release: make(chan struct{})}
	var dst bytes.Buffer
	errCh := CopyPipeAsync(ctx, &dst, r)
	cancel()
	close(r.release)
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if dst.Len() != 0 {
		t.Fatalf("expected no bytes written, got %d", dst.Len())
	}
}
