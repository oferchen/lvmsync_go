//go:build linux
// +build linux

// Package privilege implements capability detection to avoid unnecessary sudo
// usage. Only the capabilities required for LVM and direct I/O operations are
// checked.
package privilege

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// HasCaps probes the kernel for effective capabilities. Tests may replace this
// function to simulate different privilege levels.
var HasCaps = realHasCaps

// RealHasCaps returns the actual capability probe.
func RealHasCaps() bool { return realHasCaps() }

// realHasCaps probes the kernel for effective capabilities.
func realHasCaps() bool {
	hdr := &unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	data := &unix.CapUserData{}
	if err := unix.Capget(hdr, data); err != nil {
		return false
	}
	needed := []uint{unix.CAP_SYS_ADMIN, unix.CAP_SYS_RAWIO, unix.CAP_DAC_OVERRIDE}
	for _, c := range needed {
		if data.Effective&(1<<c) == 0 {
			return false
		}
	}
	return true
}

// checkCaps ensures the required capabilities are present.
func checkCaps() error {
	if HasCaps() {
		return nil
	}
	return fmt.Errorf("missing capabilities")
}

// ClearAmbient drops any ambient capabilities from the current process.
func ClearAmbient() {
	_ = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0)
}
