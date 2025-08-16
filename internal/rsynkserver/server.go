package rsynkserver

import (
	"context"
	"io"

	"lvmsync_go/internal/rsynkwire"
)

// DeviceWriter represents a block device that accepts sequential writes.
type DeviceWriter interface {
	Write([]byte) (int, error)
}

// Server consumes CRC-validated frames from a Stream and writes them to a device.
type Server struct {
	w DeviceWriter
}

// New constructs a Server that writes to the provided DeviceWriter.
func New(w DeviceWriter) *Server { return &Server{w: w} }

// Handle reads frames from stream until EOF and writes them to the DeviceWriter.
func (s *Server) Handle(ctx context.Context, stream *rsynkwire.Stream) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := s.w.Write(frame); err != nil {
			return err
		}
	}
}
