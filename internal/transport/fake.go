package transport

import "context"

// Fake is a test transport that records sent chunks and optionally returns a bitmap.
type Fake struct {
	OpenErr  error
	Chunks   []Frame
	Bitmap   []byte
	FlushErr error
	CloseErr error
}

// Open implements Transport.
func (f *Fake) Open(ctx context.Context) error {
	if f.OpenErr != nil {
		return f.OpenErr
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// SendChunk implements Transport.
func (f *Fake) SendChunk(index uint64, flags uint16, hash []byte, payload []byte) error {
	f.Chunks = append(f.Chunks, Frame{Index: index, Flags: flags, Hash: append([]byte(nil), hash...), Payload: append([]byte(nil), payload...)})
	return nil
}

// RecvBitmap implements Transport.
func (f *Fake) RecvBitmap(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return f.Bitmap, nil
	}
}

// Flush implements Transport.
func (f *Fake) Flush() error { return f.FlushErr }

// Close implements Transport.
func (f *Fake) Close() error { return f.CloseErr }

var _ Transport = (*Fake)(nil)
