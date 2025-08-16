package rsynkserver

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"lvmsync_go/internal/rsynkwire"
)

// Device represents a writable block device supporting random writes and sync.
//
// Implementations must write the provided data at the given offset and ensure
// persistence when Sync is called.
type Device interface {
	io.WriterAt
	Sync() error
}

// Server applies incoming deltas to the Device.
//
// Frames are expected to be sent using rsynkwire.Stream and consist of a leading
// byte indicating the frame type:
//
//	'S' - signature frame (currently ignored)
//	'D' - delta frame containing an 8 byte big endian offset followed by data
//
// CRC32C integrity of each frame is verified by rsynkwire.Stream.
type Server struct {
	dev Device
}

// New constructs a Server using the provided Device.
func New(dev Device) *Server { return &Server{dev: dev} }

// Handle consumes frames from the Stream until EOF, applying any delta frames to
// the Device. On graceful EOF the Device is fsynced.
func (s *Server) Handle(ctx context.Context, stream *rsynkwire.Stream) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, err := stream.Recv()
		if err == io.EOF {
			return s.dev.Sync()
		}
		if err != nil {
			return err
		}
		if len(frame) == 0 {
			continue
		}
		switch frame[0] {
		case 'S':
			// Signature frame; currently ignored.
			continue
		case 'D':
			if len(frame) < 9 {
				return fmt.Errorf("delta frame too short")
			}
			off := int64(binary.BigEndian.Uint64(frame[1:9]))
			data := frame[9:]
			if _, err := s.dev.WriteAt(data, off); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown frame type %q", frame[0])
		}
	}
}
