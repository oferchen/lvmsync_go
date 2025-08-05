package lvm2

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Runner executes LVM commands.
type Runner interface {
	Run(ctx context.Context, lvmPath string, args ...string) ([]byte, error)
}

// Client is a minimal LVM client used for tests.
type Client struct {
	lvmPath string
	runner  Runner
}

// ClientOption configures the Client.
type ClientOption func(*Client)

// NewClient constructs a new client.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{lvmPath: "/sbin/lvm"}
	for _, opt := range opts {
		opt(c)
	}
	if c.runner == nil {
		c.runner = defaultRunner{}
	}
	return c
}

// WithLVM sets the path to the LVM binary.
func WithLVM(path string) ClientOption {
	return func(c *Client) { c.lvmPath = path }
}

// WithRunner sets a custom runner for executing commands.
func WithRunner(r Runner) ClientOption {
	return func(c *Client) { c.runner = r }
}

type defaultRunner struct{}

func (defaultRunner) Run(ctx context.Context, lvmPath string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, lvmPath, args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, errOut.String())
	}
	return out.Bytes(), nil
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	return c.runner.Run(ctx, c.lvmPath, args...)
}

// CreateLVOptions contains options for creating logical volumes.
type CreateLVOptions struct {
	Name     string
	VGName   string
	Size     string
	Snapshot bool
	LVName   string
}

// RemoveLVOptions contains options for removing logical volumes.
type RemoveLVOptions struct {
	Name  string
	Force bool
}

// ListLVOptions contains options for listing logical volumes.
type ListLVOptions struct {
	Names []string
}

// ListVGOptions contains options for listing volume groups.
type ListVGOptions struct {
	Names []string
}

// LogicalVolume represents a logical volume.
type LogicalVolume struct {
	Name        string
	DataPercent string
}

// VolumeGroup represents a volume group.
type VolumeGroup struct {
	Name string
	Free string
}

// CreateLogicalVolume executes an lvcreate command.
func (c *Client) CreateLogicalVolume(ctx context.Context, opts CreateLVOptions) error {
	_, err := c.run(ctx, "lvcreate", opts.VGName, opts.LVName)
	return err
}

// RemoveLogicalVolume executes an lvremove command.
func (c *Client) RemoveLogicalVolume(ctx context.Context, opts RemoveLVOptions) error {
	_, err := c.run(ctx, "lvremove")
	return err
}

// ListLogicalVolumes executes an lvs command.
func (c *Client) ListLogicalVolumes(ctx context.Context, opts *ListLVOptions) ([]LogicalVolume, error) {
	_, err := c.run(ctx, "lvs")
	if err != nil {
		return nil, err
	}
	return []LogicalVolume{}, nil
}

// ListVolumeGroups executes a vgs command.
func (c *Client) ListVolumeGroups(ctx context.Context, opts *ListVGOptions) ([]VolumeGroup, error) {
	_, err := c.run(ctx, "vgs")
	if err != nil {
		return nil, err
	}
	return []VolumeGroup{}, nil
}
