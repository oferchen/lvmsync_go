package cgo

type VolumeGroup struct {
	Name string
	Free uint64
}

type LVM interface {
	CreateSnapshot(lvPath, snapshotName string, sizeBytes uint64) error
	RemoveLV(lvPath string) error
	SnapshotUsage(lvPath string) (float64, error)
	VGFree(vgName string) (uint64, error)
	ListVGs() ([]VolumeGroup, error)
}
