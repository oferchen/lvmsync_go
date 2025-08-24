package exitcode

import "errors"

// Exit codes for lvmsync binaries.
const (
	OK = 0
	// Precondition indicates a precondition failure before execution.
	Precondition = 2
	// Verify indicates checksum or digest verification failures.
	Verify = 3
	// Resumable indicates the process exited early but can be resumed.
	Resumable = 4
	// Config represents configuration or validation failures.
	Config = 5
	// Capability indicates missing privileges or capabilities.
	Capability = 6
)

// Sentinel errors corresponding to exit codes.
var (
	ErrPrecondition = errors.New("precondition failure")
	ErrVerify       = errors.New("verification failure")
	ErrResumable    = errors.New("resumable error")
	ErrConfig       = errors.New("configuration error")
	ErrCapability   = errors.New("capability failure")
)
