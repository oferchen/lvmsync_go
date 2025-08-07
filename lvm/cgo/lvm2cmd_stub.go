//go:build !linux || !cgo || !lvm2cmd

package cgo

import "errors"

// ErrUnsupported is returned when CGO support is unavailable.
var ErrUnsupported = errors.New("lvm unsupported")

// VolumeGroup represents a volume group with its free space in bytes.
type VolumeGroup struct {
	Name string
	Free uint64
}

// LVM defines operations provided by the CGO backend.
type LVM interface {
	CreateSnapshot(lvPath, snapshotName string, sizeBytes uint64) error
	RemoveLV(lvPath string) error
	SnapshotUsage(lvPath string) (float64, error)
	VGFree(vgName string) (uint64, error)
	ListVGs() ([]VolumeGroup, error)
}

// Cmd is a stub implementation used when CGO is unavailable.
type Cmd struct{}

// New returns a stub LVM implementation.
func New() LVM { return &Cmd{} }

func (c *Cmd) CreateSnapshot(string, string, uint64) error { return ErrUnsupported }
func (c *Cmd) RemoveLV(string) error                       { return ErrUnsupported }
func (c *Cmd) SnapshotUsage(string) (float64, error)       { return 0, ErrUnsupported }
func (c *Cmd) VGFree(string) (uint64, error)               { return 0, ErrUnsupported }
func (c *Cmd) ListVGs() ([]VolumeGroup, error)             { return nil, ErrUnsupported }
