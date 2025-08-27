package transfer

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"syscall"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func prepareTestFile(size int) (*os.File, func(), error) {
	f, err := os.CreateTemp("", "benchfile")
	if err != nil {
		return nil, nil, err
	}
	buf := make([]byte, size)
	if _, err := f.Write(buf); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, nil, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, nil, err
	}
	cleanup := func() { os.Remove(f.Name()); f.Close() }
	return f, cleanup, nil
}

func BenchmarkReadBlockWithRetriesEphemeral(b *testing.B) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		b.Fatal(err)
	}
	cfg.ZeroCopy = true
	fileSize := cfg.BlockSize * b.N
	f, cleanup, err := prepareTestFile(fileSize)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	atomic.StoreInt64(&PipeCreationCount, 0)
	for i := 0; i < b.N; i++ {
		if _, err := ReadBlockWithRetries(context.Background(), cfg, f, int64(i*cfg.BlockSize), true, [2]int{-1, -1}, zap.NewNop()); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(atomic.LoadInt64(&PipeCreationCount)), "pipes")
}

func BenchmarkReadBlockWithRetriesPersistent(b *testing.B) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		b.Fatal(err)
	}
	cfg.ZeroCopy = true
	fileSize := cfg.BlockSize * b.N
	f, cleanup, err := prepareTestFile(fileSize)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	var pipeFds [2]int
	if err := syscall.Pipe(pipeFds[:]); err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := syscall.Close(pipeFds[0]); err != nil {
			b.Fatalf("close pipe0: %v", err)
		}
	}()
	defer func() {
		if err := syscall.Close(pipeFds[1]); err != nil {
			b.Fatalf("close pipe1: %v", err)
		}
	}()

	atomic.StoreInt64(&PipeCreationCount, 0)
	for i := 0; i < b.N; i++ {
		if _, err := ReadBlockWithRetries(context.Background(), cfg, f, int64(i*cfg.BlockSize), true, pipeFds, zap.NewNop()); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(atomic.LoadInt64(&PipeCreationCount)), "pipes")
}
