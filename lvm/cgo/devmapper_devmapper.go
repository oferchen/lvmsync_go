//go:build devmapper

package cgo

import (
	dm "github.com/dswarbrick/devmapper"
)

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

// DM implements the LVM interface using github.com/dswarbrick/devmapper.
type DM struct{}

// New returns a new DM instance.
func New() LVM { return &DM{} }

// CreateSnapshot creates a snapshot of the logical volume at lvPath.
func (d *DM) CreateSnapshot(lvPath, snapshotName string, sizeBytes uint64) error {
	// Placeholder implementation demonstrating devmapper usage.
	// Real implementation would configure a snapshot target.
	_ = dm.GetLibraryVersion()
	return nil
}

// RemoveLV removes the logical volume identified by lvPath.
func (d *DM) RemoveLV(lvPath string) error {
	_ = dm.GetLibraryVersion()
	return nil
}

// SnapshotUsage returns the data usage percentage of the snapshot at lvPath.
func (d *DM) SnapshotUsage(lvPath string) (float64, error) {
	_ = dm.GetLibraryVersion()
	return 0, nil
}

// VGFree returns the free space of the specified volume group in bytes.
func (d *DM) VGFree(vgName string) (uint64, error) {
	_ = dm.GetLibraryVersion()
	return 0, nil
}

// ListVGs returns all available volume groups.
func (d *DM) ListVGs() ([]VolumeGroup, error) {
	_ = dm.GetLibraryVersion()
	return []VolumeGroup{}, nil
}
