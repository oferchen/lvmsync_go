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
	"strconv"
	"strings"
	"unsafe"
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
	dmVersion()
	vgName, lvName := filepath.Split(lvPath)
	vgName = strings.Trim(vgName, "/")

	clvm := C.cgo_lvm_init()
	if clvm == nil {
		return fmt.Errorf("lvm_init failed")
	}
	defer C.cgo_lvm_quit(clvm)

	cvg := C.cgo_vg_open(clvm, C.CString(vgName))
	if cvg == nil {
		return fmt.Errorf("vg open failed")
	}
	defer C.cgo_vg_close(cvg)

	origin := C.CString(lvName)
	snap := C.CString(snapshotName)
	defer C.free(unsafe.Pointer(origin))
	defer C.free(unsafe.Pointer(snap))

	ret := C.cgo_lv_create(cvg, origin, snap, C.uint64_t(sizeBytes))
	if ret != 0 {
		return fmt.Errorf("lvcreate failed: %d", int(ret))
	}
	return nil
}

// RemoveLV removes the logical volume identified by lvPath.
func (d *DM) RemoveLV(lvPath string) error {
	dmVersion()
	vgName, lvName := filepath.Split(lvPath)
	vgName = strings.Trim(vgName, "/")

	clvm := C.cgo_lvm_init()
	if clvm == nil {
		return fmt.Errorf("lvm_init failed")
	}
	defer C.cgo_lvm_quit(clvm)

	cvg := C.cgo_vg_open(clvm, C.CString(vgName))
	if cvg == nil {
		return fmt.Errorf("vg open failed")
	}
	defer C.cgo_vg_close(cvg)

	name := C.CString(lvName)
	defer C.free(unsafe.Pointer(name))

	ret := C.cgo_lv_remove(cvg, name)
	if ret != 0 {
		return fmt.Errorf("lvremove failed: %d", int(ret))
	}
	return nil
}

// SnapshotUsage returns the data usage percentage of the snapshot at lvPath.
func (d *DM) SnapshotUsage(lvPath string) (float64, error) {
	dmVersion()
	vgName, lvName := filepath.Split(lvPath)
	vgName = strings.Trim(vgName, "/")

	clvm := C.cgo_lvm_init()
	if clvm == nil {
		return 0, fmt.Errorf("lvm_init failed")
	}
	defer C.cgo_lvm_quit(clvm)

	cvg := C.cgo_vg_open(clvm, C.CString(vgName))
	if cvg == nil {
		return 0, fmt.Errorf("vg open failed")
	}
	defer C.cgo_vg_close(cvg)

	name := C.CString(lvName)
	defer C.free(unsafe.Pointer(name))

	var val C.double
	if C.cgo_snapshot_percent(cvg, name, &val) != 0 {
		return 0, fmt.Errorf("get snapshot usage failed")
	}
	return float64(val), nil
}

// VGFree returns the free space of the specified volume group in bytes.
func (d *DM) VGFree(vgName string) (uint64, error) {
	dmVersion()
	cName := C.CString(vgName)
	defer C.free(unsafe.Pointer(cName))

	clvm := C.cgo_lvm_init()
	if clvm == nil {
		return 0, fmt.Errorf("lvm_init failed")
	}
	defer C.cgo_lvm_quit(clvm)

	cvg := C.cgo_vg_open(clvm, cName)
	if cvg == nil {
		return 0, fmt.Errorf("vg open failed")
	}
	defer C.cgo_vg_close(cvg)

	free := C.cgo_vg_free(cvg)
	return uint64(free), nil
}

// ListVGs returns all available volume groups.
func (d *DM) ListVGs() ([]VolumeGroup, error) {
	dmVersion()
	clvm := C.cgo_lvm_init()
	if clvm == nil {
		return nil, fmt.Errorf("lvm_init failed")
	}
	defer C.cgo_lvm_quit(clvm)

	list := C.cgo_list_vgs(clvm)
	var vgs []VolumeGroup
	for item := list; item != nil; item = item.next {
		name := C.cgo_list_item_str(item)
		if name == nil {
			continue
		}
		vgName := C.GoString(name)
		cvg := C.cgo_vg_open(clvm, C.CString(vgName))
		if cvg == nil {
			continue
		}
		free := C.cgo_vg_free(cvg)
		C.cgo_vg_close(cvg)
		vgs = append(vgs, VolumeGroup{Name: vgName, Free: uint64(free)})
	}
	return vgs, nil
}
