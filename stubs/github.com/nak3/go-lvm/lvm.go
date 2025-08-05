package lvm

// Stub implementation of github.com/nak3/go-lvm used for tests.

// VgHandle simulates a handle to a volume group.
type VgHandle struct{ Name string }

// LvHandle simulates a handle to a logical volume.
type LvHandle struct {
	Name   string
	Parent *VgHandle
}

// VgObject represents a volume group.
type VgObject struct{ Vgt *VgHandle }

// LvObject represents a logical volume.
type LvObject struct {
	Lvt      *LvHandle
	parentVG *VgObject
}

// Properties mirrors the real library's property type.
type Properties struct {
	SignedInteger int
	Integer       int
	Str           string
}

var vgFree = map[string]uint64{
	"vg0": 1024,
	"vg1": 2048,
}

// ListVgNames returns available volume group names.
func ListVgNames() []string {
	names := make([]string, 0, len(vgFree))
	for n := range vgFree {
		names = append(names, n)
	}
	return names
}

// VgOpen opens a volume group.
func VgOpen(name, mode string) *VgHandle {
	return &VgHandle{Name: name}
}

// Close closes the volume group.
func (v *VgObject) Close() error { return nil }

// LvFromName returns an LV object by name.
func (v *VgObject) LvFromName(name string) (*LvObject, error) {
	return &LvObject{Lvt: &LvHandle{Name: name, Parent: v.Vgt}, parentVG: v}, nil
}

// GetFreeSize returns the free size of the volume group.
func (v *VgObject) GetFreeSize() uint64 {
	if s, ok := vgFree[v.Vgt.Name]; ok {
		return s
	}
	return 0
}

// Snapshot creates a snapshot of the logical volume.
func (l *LvObject) Snapshot(name string, size uint64) (*LvObject, error) {
	return &LvObject{Lvt: &LvHandle{Name: name, Parent: l.parentVG.Vgt}, parentVG: l.parentVG}, nil
}

// Remove removes the logical volume.
func (l *LvObject) Remove() error { return nil }

// GetProperty returns a property of the logical volume.
func (l *LvObject) GetProperty(name string) (Properties, error) {
	if name == "data_percent" {
		return Properties{Str: "55.5"}, nil
	}
	return Properties{}, nil
}
