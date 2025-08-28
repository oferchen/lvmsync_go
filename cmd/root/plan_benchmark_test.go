package root

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"

	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/manifest"
)

func BenchmarkEstimateBytes(b *testing.B) {
	dir := b.TempDir()
	file, err := os.CreateTemp(dir, "src-*.bin")
	if err != nil {
		b.Fatalf("tempfile: %v", err)
	}
	data1 := make([]byte, 4096)
	data2 := make([]byte, 4096)
	r := rand.New(rand.NewSource(1))
	r.Read(data1)
	r.Read(data2)
	if _, err := file.Write(append(data1, data2...)); err != nil {
		b.Fatalf("write: %v", err)
	}
	file.Close()

	manPath := filepath.Join(dir, "src.man")
	idx, err := manifest.Create(manPath, "dev", 8192, 0, 0, 0, 4096, 0, 0, 0, 0)
	if err != nil {
		b.Fatalf("manifest create: %v", err)
	}
	if err := idx.Set(0, 4096, 0, xxh3.Hash(data1), blake3.Sum256(data1)); err != nil {
		b.Fatalf("manifest set1: %v", err)
	}
	if err := idx.Set(4096, 4096, 0, xxh3.Hash(data2), blake3.Sum256(data2)); err != nil {
		b.Fatalf("manifest set2: %v", err)
	}
	if err := idx.Close(); err != nil {
		b.Fatalf("manifest close: %v", err)
	}

	cfg := &config.Config{ManifestPath: manPath}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := estimateBytes(file.Name(), cfg); err != nil {
			b.Fatalf("estimateBytes: %v", err)
		}
	}
}
