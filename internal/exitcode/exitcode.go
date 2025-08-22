package exitcode

import "errors"

// Exit codes for lvmsync binaries.
const (
	OK = 0
	// Capability indicates missing privileges or capabilities.
	Capability = 10
	// Device indicates a device-related failure.
	Device = 20
	// SnapshotExhausted indicates snapshot space was exhausted during transfer.
	SnapshotExhausted = 25
	// Platform is returned when the operating system is unsupported.
	Platform = 30
	// Config represents configuration or validation failures.
	Config = 40
	// Runtime represents errors during command execution.
	Runtime = 50
	// Verify indicates checksum or digest verification failures.
	Verify = 60
	// Partial indicates a resumable transfer was aborted before completion.
	Partial = 70
	// Precondition indicates a precondition failure before execution.
	Precondition = 80
	// Resumable indicates the process exited early but can be resumed.
	Resumable = 90
)

// Sentinel errors corresponding to exit codes.
var (
	ErrCapability        = errors.New("capability failure")
	ErrDevice            = errors.New("device failure")
	ErrSnapshotExhausted = errors.New("snapshot exhausted")
	ErrPlatform          = errors.New("unsupported platform")
	ErrConfig            = errors.New("configuration error")
	ErrRuntime           = errors.New("runtime error")
	ErrVerify            = errors.New("verification failure")
	ErrPartial           = errors.New("partial transfer")
	ErrPrecondition      = errors.New("precondition failure")
	ErrResumable         = errors.New("resumable error")
)
