//go:build linux && cgo && lvm2cmd

package cgo

/*
#cgo pkg-config: lvm2
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <lvm2cmd.h>

static char *out_buf = NULL;
static size_t out_len = 0;

static void log_acc(int level, const char *file, int line, int dm_errno, const char *message) {
    if (level != LVM2_LOG_PRINT && level != LVM2_LOG_ERROR && level != LVM2_LOG_FATAL)
        return;
    size_t len = strlen(message);
    char *tmp = realloc(out_buf, out_len + len + 2);
    if (!tmp)
        return;
    out_buf = tmp;
    memcpy(out_buf + out_len, message, len);
    out_len += len;
    out_buf[out_len++] = '\n';
    out_buf[out_len] = '\0';
}

static void reset_log() {
    if (out_buf) {
        free(out_buf);
        out_buf = NULL;
    }
    out_len = 0;
}

static const char* get_log() {
    return out_buf ? out_buf : "";
}

static int run_cmd(const char *cmd) {
    reset_log();
    lvm2_log_fn(log_acc);
    return lvm2_run(NULL, cmd);
}
*/
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

// VolumeGroup represents a volume group with its free space in bytes.
// Cmd implements LVM using liblvm2cmd.
type Cmd struct{}

// New returns a new Cmd instance.
func New() LVM { return &Cmd{} }

func (c *Cmd) run(args []string) (string, error) {
	cmd := strings.Join(args, " ")
	ccmd := C.CString(cmd)
	defer C.free(unsafe.Pointer(ccmd))
	ret := C.run_cmd(ccmd)
	out := C.GoString(C.get_log())
	if int(ret) != int(C.LVM2_COMMAND_SUCCEEDED) {
		if out == "" {
			return "", fmt.Errorf("%s failed with code %d", cmd, int(ret))
		}
		return "", fmt.Errorf("%s failed: %s", cmd, strings.TrimSpace(out))
	}
	return out, nil
}

// CreateSnapshot creates a snapshot of lvPath.
func (c *Cmd) CreateSnapshot(lvPath, snapshotName string, sizeBytes uint64) error {
	size := fmt.Sprintf("%dB", sizeBytes)
	_, err := c.run([]string{"lvcreate", "-s", "-n", snapshotName, "-L", size, lvPath})
	return err
}

// RemoveLV removes the logical volume at lvPath.
func (c *Cmd) RemoveLV(lvPath string) error {
	_, err := c.run([]string{"lvremove", "-y", lvPath})
	return err
}

// SnapshotUsage returns snapshot data usage percentage.
func (c *Cmd) SnapshotUsage(lvPath string) (float64, error) {
	out, err := c.run([]string{"lvs", "--noheadings", "--units", "b", "--nosuffix", "-o", "data_percent", lvPath})
	if err != nil {
		return 0, err
	}
	val := strings.TrimSpace(out)
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, fmt.Errorf("parse snapshot usage %q: %w", val, err)
	}
	return f, nil
}

// VGFree returns free bytes for the volume group.
func (c *Cmd) VGFree(vgName string) (uint64, error) {
	out, err := c.run([]string{"vgs", "--noheadings", "--units", "b", "--nosuffix", "-o", "vg_free", vgName})
	if err != nil {
		return 0, err
	}
	val := strings.TrimSpace(out)
	u, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse free space %q: %w", val, err)
	}
	return u, nil
}

// ListVGs lists all volume groups and their free bytes.
func (c *Cmd) ListVGs() ([]VolumeGroup, error) {
	out, err := c.run([]string{"vgs", "--noheadings", "--units", "b", "--nosuffix", "-o", "vg_name,vg_free"})
	if err != nil {
		return nil, err
	}
	var vgs []VolumeGroup
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
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
