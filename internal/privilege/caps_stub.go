//go:build !linux
// +build !linux

// Package privilege provides stubs for capability checks on non-Linux systems.
package privilege

// HasCaps reports whether the required capabilities are present.
// Non-Linux platforms do not support these capabilities, so HasCaps always
// returns false.
var HasCaps = realHasCaps

// RealHasCaps returns false on non-Linux platforms.
func RealHasCaps() bool { return realHasCaps() }

func realHasCaps() bool { return false }

// checkCaps is a no-op on non-Linux platforms since capabilities are
// unsupported and sudo will be required.
func checkCaps() error { return nil }
