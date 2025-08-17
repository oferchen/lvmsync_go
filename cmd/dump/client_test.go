package dump

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"lvmsync_go/internal/config"
	"lvmsync_go/transfer"
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

type countingSyncCore struct {
	zapcore.Core
	count int
}

func (c *countingSyncCore) Sync() error {
	c.count++
	return nil
}

func TestRunSyncsLogger(t *testing.T) {
	cfg := &config.Config{StdoutMode: true, Parallel: 1, BlockSize: 4096, DedupStrategy: "none"}
	core := &countingSyncCore{Core: zapcore.NewNopCore()}
	logger := zap.New(core)
	origSeq := dumpChangesSequential
	dumpChangesSequential = func(context.Context, *transfer.Transfer, *config.Config, string, string, io.Writer) error { return nil }
	defer func() { dumpChangesSequential = origSeq }()
	snap := filepath.Join(t.TempDir(), "snap")
	if err := os.WriteFile(snap, []byte("data"), 0o600); err != nil {
		t.Fatalf("write snap: %v", err)
	}
	if _, err := Run(context.Background(), cfg, snap, "", logger); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if core.count != 1 {
		t.Fatalf("expected Sync to be called once, got %d", core.count)
	}
}
