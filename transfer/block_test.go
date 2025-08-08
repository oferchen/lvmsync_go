package transfer

import (
	"bytes"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"lvmsync_go/config"

	"go.uber.org/zap"
)

func TestReadBlockWithRetriesTransientFailure(t *testing.T) {
	SetLogger(zap.NewNop())

	blockSize := 4
	data := []byte{1, 2, 3, 4}
	cfg := &config.Config{BlockSize: blockSize, MaxRetries: 3}

	tmp := newTempFile(t, "block")
	defer tmp.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		if _, err := tmp.WriteAt(data, 0); err != nil {
			t.Logf("write failed: %v", err)
		}
	}()

	start := time.Now()
	buf, err := ReadBlockWithRetries(cfg, tmp, 0, false, [2]int{-1, -1})
	if err != nil {
		t.Fatalf("ReadBlockWithRetries returned error: %v", err)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("unexpected data: %v", buf)
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatalf("expected at least one retry, duration %v", time.Since(start))
	}
	putBlockBuffer(buf)
}

func TestReadBlockWithRetriesPipeHandling(t *testing.T) {
	SetLogger(zap.NewNop())

	blockSize := 4
	data := []byte{1, 2, 3, 4}
	cfg := &config.Config{BlockSize: blockSize, MaxRetries: 1}

	tmp := newTempFile(t, "block")
	defer tmp.Close()
	if _, err := tmp.WriteAt(data, 0); err != nil {
		t.Fatalf("write: %v", err)
	}

	atomic.StoreInt64(&PipeCreationCount, 0)
	buf, err := ReadBlockWithRetries(cfg, tmp, 0, true, [2]int{-1, -1})
	if err != nil {
		t.Fatalf("ReadBlockWithRetries error: %v", err)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("unexpected data: %v", buf)
	}
	if c := atomic.LoadInt64(&PipeCreationCount); c != 1 {
		t.Fatalf("expected PipeCreationCount 1, got %d", c)
	}
	putBlockBuffer(buf)

	fds := [2]int{}
	if err := syscall.Pipe(fds[:]); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() {
		if err := syscall.Close(fds[0]); err != nil {
			t.Logf("close fd0: %v", err)
		}
	}()
	defer func() {
		if err := syscall.Close(fds[1]); err != nil {
			t.Logf("close fd1: %v", err)
		}
	}()

	atomic.StoreInt64(&PipeCreationCount, 0)
	buf, err = ReadBlockWithRetries(cfg, tmp, 0, true, fds)
	if err != nil {
		t.Fatalf("ReadBlockWithRetries error: %v", err)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("unexpected data: %v", buf)
	}
	if c := atomic.LoadInt64(&PipeCreationCount); c != 0 {
		t.Fatalf("expected PipeCreationCount 0, got %d", c)
	}
	putBlockBuffer(buf)
}
