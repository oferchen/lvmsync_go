package quic

import (
	"context"

	q "github.com/quic-go/quic-go"
)

// StreamSet holds the control stream and a set of data streams.
type StreamSet struct {
	Control q.SendStream
	Data    []q.Stream
}

// OpenStreams opens a control stream for manifest/bitmap/acks and n
// bidirectional data streams for chunk transfer.
func OpenStreams(ctx context.Context, conn q.Connection, n int) (*StreamSet, error) {
	ctrl, err := conn.OpenUniStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	ss := &StreamSet{Control: ctrl}
	for i := 0; i < n; i++ {
		s, err := conn.OpenStreamSync(ctx)
		if err != nil {
			_ = ctrl.Close()
			for _, ds := range ss.Data {
				_ = ds.Close()
			}
			return nil, err
		}
		ss.Data = append(ss.Data, s)
	}
	return ss, nil
}
