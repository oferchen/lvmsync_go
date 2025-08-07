//go:build linux && cgo

package cgo

/*
#cgo pkg-config: lvm2app devmapper
#include <stdlib.h>
#include <lvm2cmd.h>
#include <libdevmapper.h>

extern void goLog(int level, const char *file, int line, int dm_errno, const char *message);

static void bridge_log(int level, const char *file, int line, int dm_errno, const char *message) {
    goLog(level, file, line, dm_errno, message);
}
*/
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
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

var (
	logMu  sync.Mutex
	logBuf strings.Builder
)

//export goLog
func goLog(level C.int, file *C.char, line C.int, dmErrno C.int, message *C.char) {
	if level != C.LVM2_LOG_PRINT && level != C.LVM2_LOG_ERROR {
		return
	}
	if message == nil {
		return
	}
	logBuf.WriteString(C.GoString(message))
	logBuf.WriteByte('\n')
}

func runLVM(args ...string) (string, error) {
	cmd := strings.Join(args, " ")
	ccmd := C.CString(cmd)
	defer C.free(unsafe.Pointer(ccmd))

	logMu.Lock()
	logBuf.Reset()
	C.lvm2_log_fn((C.lvm2_log_fn_t)(unsafe.Pointer(C.bridge_log)))
	ret := C.lvm2_run(nil, ccmd)
	out := logBuf.String()
	logMu.Unlock()

	if ret != C.LVM2_COMMAND_SUCCEEDED {
		return out, fmt.Errorf("lvm command failed: %s", strings.TrimSpace(out))
	}
	return out, nil
}

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
	size := fmt.Sprintf("%dB", sizeBytes)
	_, err := runLVM("lvcreate", "--snapshot", "--name", snapshotName, "--size", size, lvPath)
	if err != nil {
		return err
	}
	return nil
}

// RemoveLV removes the logical volume identified by lvPath.
func (d *DM) RemoveLV(lvPath string) error {
	dmVersion()
	_, err := runLVM("lvremove", "-f", lvPath)
	return err
}

// SnapshotUsage returns the data usage percentage of the snapshot at lvPath.
func (d *DM) SnapshotUsage(lvPath string) (float64, error) {
	dmVersion()
	out, err := runLVM("lvs", "--noheadings", "--units", "b", "--nosuffix", "-o", "data_percent", lvPath)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0, fmt.Errorf("unable to parse lvs output: %q", out)
	}
	val, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}

// VGFree returns the free space of the specified volume group in bytes.
func (d *DM) VGFree(vgName string) (uint64, error) {
	dmVersion()
	out, err := runLVM("vgs", "--noheadings", "--units", "b", "--nosuffix", "-o", "vg_free", vgName)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0, fmt.Errorf("unable to parse vgs output: %q", out)
	}
	val, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}

// ListVGs returns all available volume groups.
func (d *DM) ListVGs() ([]VolumeGroup, error) {
	dmVersion()
	out, err := runLVM("vgs", "--noheadings", "--units", "b", "--nosuffix", "-o", "vg_name,vg_free")
	if err != nil {
		return nil, err
	}
	var vgs []VolumeGroup
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		free, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		vgs = append(vgs, VolumeGroup{Name: fields[0], Free: free})
	}
	return vgs, nil
}
