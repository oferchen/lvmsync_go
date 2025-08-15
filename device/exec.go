//go:build linux

package device

import "os/exec"

// execCommand is a helper for running external commands. It can be stubbed in tests.
var execCommand = exec.CommandContext
