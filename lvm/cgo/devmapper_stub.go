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

// Conn is a stub connection returned when CGO is unavailable.
type Conn struct{}

// Open returns ErrUnsupported when CGO is unavailable.
func Open() (*Conn, error) { return nil, ErrUnsupported }

// Close is a no-op for the stub.
func (c *Conn) Close() {}

// VG is a stub volume group handle.
type VG struct{}

// OpenVG returns ErrUnsupported when CGO is unavailable.
func (c *Conn) OpenVG(string) (*VG, error) { return nil, ErrUnsupported }

// Close returns ErrUnsupported for the stub.
func (v *VG) Close() error { return ErrUnsupported }

// FreeBytes returns zero for the stub.
func (v *VG) FreeBytes() uint64 { return 0 }

// CreateSnapshot always returns ErrUnsupported.
func (v *VG) CreateSnapshot(string, string, uint64) error { return ErrUnsupported }

// RemoveLV always returns ErrUnsupported.
func (v *VG) RemoveLV(string) error { return ErrUnsupported }

// SnapshotUsage always returns ErrUnsupported.
func (v *VG) SnapshotUsage(string) (float64, error) { return 0, ErrUnsupported }

// ListVolumeGroups always returns ErrUnsupported.
func (c *Conn) ListVolumeGroups() ([]VolumeGroup, error) { return nil, ErrUnsupported }

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
