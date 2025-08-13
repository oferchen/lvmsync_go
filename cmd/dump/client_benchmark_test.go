package dump

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func BenchmarkCopyPipeAsync(b *testing.B) {
	data := bytes.Repeat([]byte("a"), 1<<20) // 1 MiB
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		src := bytes.NewReader(data)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		errCh := CopyPipeAsync(ctx, io.Discard, src)
		cancel()
		if err := <-errCh; err != nil {
			b.Fatal(err)
		}
	}
}
