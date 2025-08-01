package transfer

import (
	"os"
	"sync/atomic"
	"testing"

	"lvmsync_go/config"
	"syscall"
)

func makeCleanupFunc(f *os.File) func() {
	return func() { os.Remove(f.Name()); f.Close() }
}

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
	return f, makeCleanupFunc(f), nil
}

func BenchmarkReadBlockWithRetriesEphemeral(b *testing.B) {
	cfg := config.DefaultConfig()
	cfg.ZeroCopy = true
	fileSize := cfg.BlockSize * b.N
	f, cleanup, err := prepareTestFile(fileSize)
	if err != nil {
		b.Fatal(err)
	}
	defer cleanup()

	atomic.StoreInt64(&PipeCreationCount, 0)
	for i := 0; i < b.N; i++ {
		if _, err := ReadBlockWithRetries(cfg, f, int64(i*cfg.BlockSize), true, [2]int{-1, -1}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(atomic.LoadInt64(&PipeCreationCount)), "pipes")
}

func BenchmarkReadBlockWithRetriesPersistent(b *testing.B) {
	cfg := config.DefaultConfig()
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
	defer syscall.Close(pipeFds[0])
	defer syscall.Close(pipeFds[1])

	atomic.StoreInt64(&PipeCreationCount, 0)
	for i := 0; i < b.N; i++ {
		if _, err := ReadBlockWithRetries(cfg, f, int64(i*cfg.BlockSize), true, pipeFds); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(atomic.LoadInt64(&PipeCreationCount)), "pipes")
}
