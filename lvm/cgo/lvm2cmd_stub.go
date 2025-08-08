//go:build !linux || !cgo || !lvm2cmd

package cgo

import "errors"

// ErrUnsupported is returned when CGO support is unavailable.
var ErrUnsupported = errors.New("lvm unsupported")

// Cmd is a stub implementation used when CGO is unavailable.
type Cmd struct{}

// New returns a stub LVM implementation.
func New() LVM { return &Cmd{} }

func (c *Cmd) CreateSnapshot(string, string, uint64) error { return ErrUnsupported }
func (c *Cmd) RemoveLV(string) error                       { return ErrUnsupported }
func (c *Cmd) SnapshotUsage(string) (float64, error)       { return 0, ErrUnsupported }
func (c *Cmd) VGFree(string) (uint64, error)               { return 0, ErrUnsupported }
func (c *Cmd) ListVGs() ([]VolumeGroup, error)             { return nil, ErrUnsupported }
