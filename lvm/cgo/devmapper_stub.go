//go:build !linux || !cgo

package cgo

import "errors"

// ErrUnsupported is returned when CGO support is unavailable.
var ErrUnsupported = errors.New("lvm unsupported")

// VolumeGroup represents a volume group with its free space in bytes.
type VolumeGroup struct {
	Name string
	Free uint64
}

// LVM provides access to LVM operations via device-mapper.
type LVM interface {
	CreateSnapshot(lvPath, snapshotName string, sizeBytes uint64) error
	RemoveLV(lvPath string) error
	SnapshotUsage(lvPath string) (float64, error)
	VGFree(vgName string) (uint64, error)
	ListVGs() ([]VolumeGroup, error)
}

// DM is a stub implementation used when CGO is unavailable.
type DM struct{}

// New returns a stub DM implementation.
func New() LVM { return &DM{} }

func (d *DM) CreateSnapshot(lvPath, snapshotName string, sizeBytes uint64) error {
	return ErrUnsupported
}

func (d *DM) RemoveLV(lvPath string) error {
	return ErrUnsupported
}

func (d *DM) SnapshotUsage(lvPath string) (float64, error) {
	return 0, ErrUnsupported
}

func (d *DM) VGFree(vgName string) (uint64, error) {
	return 0, ErrUnsupported
}

func (d *DM) ListVGs() ([]VolumeGroup, error) {
	return nil, ErrUnsupported
}
