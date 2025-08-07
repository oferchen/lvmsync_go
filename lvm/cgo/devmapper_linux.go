//go:build linux && cgo

package cgo

/*
#cgo pkg-config: lvm2 devmapper
#include <stdlib.h>
#include <stdint.h>
#include <libdevmapper.h>
#include <liblvm.h>

// Thin wrappers around liblvm2 used by the Go code.  These helpers are
// intentionally minimal and return raw status codes so that Go can
// translate them into idiomatic errors.

static lvm_t cgo_lvm_init() {
    return lvm_init(NULL);
}

static void cgo_lvm_quit(lvm_t lvm) {
    lvm_quit(lvm);
}

static vg_t cgo_vg_open(lvm_t lvm, const char *name) {
    return lvm_vg_open(lvm, name, "w", 0);
}

static int cgo_vg_close(vg_t vg) {
    return lvm_vg_close(vg);
}

static int cgo_lv_create(vg_t vg, const char *origin, const char *snap, uint64_t size) {
    struct lvcreate_params params = {0};
    params.lv_name = snap;
    params.origin_name = origin;
    params.size = size;
    return lvm_lv_create(vg, &params);
}

static int cgo_lv_remove(vg_t vg, const char *name) {
    lv_t lv = lvm_lv_from_name(vg, name);
    if (!lv) {
        return -1;
    }
    return lvm_lv_remove(lv);
}

static uint64_t cgo_vg_free(vg_t vg) {
    return lvm_vg_get_free(vg);
}

static struct dm_list *cgo_list_vgs(lvm_t lvm) {
    return lvm_list_vg_names(lvm);
}

static const char *cgo_list_item_str(struct dm_list *item) {
    return (const char *)item->data;
}

static int cgo_snapshot_percent(vg_t vg, const char *name, double *out) {
    lv_t lv = lvm_lv_from_name(vg, name);
    if (!lv)
        return -1;
    struct lvm_property_value v = lvm_lv_get_property(lv, "data_percent");
    if (!v.is_valid)
        return -1;
    *out = v.value.d;
    return 0;
}

*/
import "C"

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"
)

// VolumeGroup represents a volume group with its free space in bytes.
type VolumeGroup struct {
	Name string
	Free uint64
}

// Conn represents an initialized liblvm handle.
type Conn struct {
	lvm C.lvm_t
}

// Open initializes a new liblvm connection.
func Open() (*Conn, error) {
	dmVersion()
	lvm := C.cgo_lvm_init()
	if lvm == nil {
		return nil, fmt.Errorf("lvm_init failed")
	}
	return &Conn{lvm: lvm}, nil
}

// Close releases the underlying liblvm handle.
func (c *Conn) Close() {
	if c.lvm != nil {
		C.cgo_lvm_quit(c.lvm)
		c.lvm = nil
	}
}

// VG represents an opened volume group.
type VG struct {
	conn *Conn
	vg   C.vg_t
	name string
}

// OpenVG opens the named volume group.
func (c *Conn) OpenVG(name string) (*VG, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	vg := C.cgo_vg_open(c.lvm, cName)
	if vg == nil {
		return nil, fmt.Errorf("vg open failed")
	}
	return &VG{conn: c, vg: vg, name: name}, nil
}

// Close releases the volume group handle.
func (v *VG) Close() error {
	if v.vg == nil {
		return nil
	}
	if C.cgo_vg_close(v.vg) != 0 {
		return fmt.Errorf("vg close failed")
	}
	v.vg = nil
	return nil
}

// FreeBytes returns free space in bytes for the volume group.
func (v *VG) FreeBytes() uint64 { return uint64(C.cgo_vg_free(v.vg)) }

// CreateSnapshot creates a snapshot of origin with the given name and size.
func (v *VG) CreateSnapshot(origin, snapshot string, size uint64) error {
	corigin := C.CString(origin)
	csnap := C.CString(snapshot)
	defer C.free(unsafe.Pointer(corigin))
	defer C.free(unsafe.Pointer(csnap))
	if ret := C.cgo_lv_create(v.vg, corigin, csnap, C.uint64_t(size)); ret != 0 {
		return fmt.Errorf("lvcreate failed: %d", int(ret))
	}
	return nil
}

// RemoveLV removes the logical volume with the given name.
func (v *VG) RemoveLV(name string) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	if ret := C.cgo_lv_remove(v.vg, cname); ret != 0 {
		return fmt.Errorf("lvremove failed: %d", int(ret))
	}
	return nil
}

// SnapshotUsage returns the data usage percentage of the snapshot.
func (v *VG) SnapshotUsage(name string) (float64, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var val C.double
	if C.cgo_snapshot_percent(v.vg, cname, &val) != 0 {
		return 0, fmt.Errorf("get snapshot usage failed")
	}
	return float64(val), nil
}

// ListVolumeGroups returns all available volume groups.
func (c *Conn) ListVolumeGroups() ([]VolumeGroup, error) {
	list := C.cgo_list_vgs(c.lvm)
	var vgs []VolumeGroup
	for item := list; item != nil; item = item.next {
		name := C.cgo_list_item_str(item)
		if name == nil {
			continue
		}
		vgName := C.GoString(name)
		vg, err := c.OpenVG(vgName)
		if err != nil {
			continue
		}
		free := vg.FreeBytes()
		vg.Close()
		vgs = append(vgs, VolumeGroup{Name: vgName, Free: free})
	}
	return vgs, nil
}

// LVM provides access to LVM operations via device-mapper.
type LVM interface {
	CreateSnapshot(lvPath, snapshotName string, sizeBytes uint64) error
	RemoveLV(lvPath string) error
	SnapshotUsage(lvPath string) (float64, error)
	VGFree(vgName string) (uint64, error)
	ListVGs() ([]VolumeGroup, error)
}

// DM implements the LVM interface using direct liblvm2/libdevmapper calls.
type DM struct{}

// New returns a new DM instance.
func New() LVM { return &DM{} }

// dmVersion calls into libdevmapper to ensure linkage.
func dmVersion() string {
	var buf [128]C.char
	if C.dm_get_library_version(&buf[0], 128) == 0 {
		return ""
	}
	return C.GoString(&buf[0])
}

// CreateSnapshot creates a snapshot of the logical volume at lvPath.
func (d *DM) CreateSnapshot(lvPath, snapshotName string, sizeBytes uint64) error {
	vgName, lvName := filepath.Split(lvPath)
	vgName = strings.Trim(vgName, "/")

	conn, err := Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	vg, err := conn.OpenVG(vgName)
	if err != nil {
		return err
	}
	defer vg.Close()

	return vg.CreateSnapshot(lvName, snapshotName, sizeBytes)
}

// RemoveLV removes the logical volume identified by lvPath.
func (d *DM) RemoveLV(lvPath string) error {
	vgName, lvName := filepath.Split(lvPath)
	vgName = strings.Trim(vgName, "/")

	conn, err := Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	vg, err := conn.OpenVG(vgName)
	if err != nil {
		return err
	}
	defer vg.Close()

	return vg.RemoveLV(lvName)
}

// SnapshotUsage returns the data usage percentage of the snapshot at lvPath.
func (d *DM) SnapshotUsage(lvPath string) (float64, error) {
	vgName, lvName := filepath.Split(lvPath)
	vgName = strings.Trim(vgName, "/")

	conn, err := Open()
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	vg, err := conn.OpenVG(vgName)
	if err != nil {
		return 0, err
	}
	defer vg.Close()

	return vg.SnapshotUsage(lvName)
}

// VGFree returns the free space of the specified volume group in bytes.
func (d *DM) VGFree(vgName string) (uint64, error) {
	conn, err := Open()
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	vg, err := conn.OpenVG(vgName)
	if err != nil {
		return 0, err
	}
	defer vg.Close()

	return vg.FreeBytes(), nil
}

// ListVGs returns all available volume groups.
func (d *DM) ListVGs() ([]VolumeGroup, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return conn.ListVolumeGroups()
}
