package rsynkwire

import (
	"context"
	"io"

	"github.com/gokrazy/rsync/rsyncclient"
)

// Stream represents a bidirectional byte stream used by the rsync protocol.
type Stream interface {
	io.Reader
	io.Writer
}

// Client wraps gokrazy/rsync's Client and exposes a Stream-based Run method.
type Client struct {
	*rsyncclient.Client
}

// NewClient constructs a new Client using the provided rsync arguments and options.
func NewClient(args []string, opts ...rsyncclient.Option) (*Client, error) {
	c, err := rsyncclient.New(args, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{Client: c}, nil
}

// Run executes the rsync protocol over the specified Stream.
//
// The paths parameter specifies the local paths when the client acts as a sender
// or the destination when acting as a receiver, mirroring rsync's semantics.
func (c *Client) Run(ctx context.Context, s Stream, paths []string) (*rsyncclient.Result, error) {
	return c.Client.Run(ctx, s, paths)
}
