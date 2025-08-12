package dump

import (
    "bytes"
    "context"
    "io"
    "testing"
)

func BenchmarkCopyPipeAsync(b *testing.B) {
    data := bytes.Repeat([]byte("a"), 1<<20) // 1 MiB
    b.SetBytes(int64(len(data)))
    for i := 0; i < b.N; i++ {
        src := bytes.NewReader(data)
        errCh := CopyPipeAsync(context.Background(), io.Discard, src)
        if err := <-errCh; err != nil {
            b.Fatal(err)
        }
    }
}

