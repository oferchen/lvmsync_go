package common

import (
	"context"
	"os"
)

// OpenWithContext opens the file at path respecting ctx cancellation.
// It returns context error if ctx is done before the open completes.
func OpenWithContext(ctx context.Context, path string) (*os.File, error) {
	if ctx == nil {
		return os.Open(path)
	}
	type result struct {
		f   *os.File
		err error
	}
	ch := make(chan result, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_RDONLY, 0)
		ch <- result{f: f, err: err}
	}()
	select {
	case <-ctx.Done():
		go func() {
			r := <-ch
			if r.err == nil {
				_ = r.f.Close()
			}
		}()
		return nil, ctx.Err()
	case r := <-ch:
		return r.f, r.err
	}
}
