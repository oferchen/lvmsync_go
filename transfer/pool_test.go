package transfer

import (
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

func BenchmarkReadBlockWithPool(b *testing.B) {
	cfg := &config.Config{BlockSize: 4096, MaxRetries: 1}
	f, err := os.CreateTemp("", "poolbench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(f.Name())
	data := make([]byte, cfg.BlockSize)
	if _, err = f.Write(data); err != nil {
		b.Fatal(err)
	}
	f.Close()
	f, err = os.Open(f.Name())
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, err := ReadBlockWithRetries(cfg, f, 0, false, [2]int{-1, -1}, zap.NewNop())
		if err != nil {
			b.Fatal(err)
		}
		putBlockBuffer(buf)
	}
}

func BenchmarkReadBlockWithMake(b *testing.B) {
	cfg := &config.Config{BlockSize: 4096, MaxRetries: 1}
	f, err := os.CreateTemp("", "poolbench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(f.Name())
	data := make([]byte, cfg.BlockSize)
	if _, err = f.Write(data); err != nil {
		b.Fatal(err)
	}
	f.Close()
	f, err = os.Open(f.Name())
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	b.ReportAllocs()
	b.ResetTimer()
	buf := make([]byte, cfg.BlockSize)
	for i := 0; i < b.N; i++ {
		if _, err = f.ReadAt(buf, 0); err != nil {
			b.Fatal(err)
		}
		buf = make([]byte, cfg.BlockSize)
	}
}
