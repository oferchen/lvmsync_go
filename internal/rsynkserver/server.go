package rsynkserver

import (
	"context"
	"io"

	"github.com/gokrazy/rsync/rsyncd"

	"lvmsync_go/internal/rsynkwire"
)

// Server provides a minimal wrapper around gokrazy/rsync's rsyncd server
// operating on a Stream.
type Server struct {
	srv *rsyncd.Server
}

// New constructs a new Server instance. Filesystem restrictions are disabled and
// all stderr output is discarded so that the server can be embedded.
func New() (*Server, error) {
	srv, err := rsyncd.NewServer(nil, rsyncd.DontRestrict(), rsyncd.WithStderr(io.Discard))
	if err != nil {
		return nil, err
	}
	return &Server{srv: srv}, nil
}

// Handle processes a single rsync session over the provided Stream. The args
// parameter should come from rsyncclient.Client.ServerCommandOptions.
func (s *Server) Handle(ctx context.Context, stream rsynkwire.Stream, args []string) error {
	conn := rsyncd.NewConnection(stream, stream, "<stream>")
	// HandleConnArgs performs argument parsing internally using rsync's own
	// facilities, avoiding the need to depend on its internal packages.
	return s.srv.HandleConnArgs(ctx, conn, nil, args)
}
