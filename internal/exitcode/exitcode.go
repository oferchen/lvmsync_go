package exitcode

// Exit codes for lvmsync binaries.
const (
	OK = 0
	// ErrCapability indicates missing privileges or capabilities.
	ErrCapability = 10
	// ErrDevice indicates a device-related failure.
	ErrDevice = 20
	// ErrPlatform is returned when the operating system is unsupported.
	ErrPlatform = 30
	// ErrConfig represents configuration or validation failures.
	ErrConfig = 40
	// ErrRuntime represents errors during command execution.
	ErrRuntime = 50
	// ErrVerify indicates checksum or digest verification failures.
	ErrVerify = 60
	// ErrPartial indicates a resumable transfer was aborted before completion.
	ErrPartial = 70
	// ErrPrecondition indicates a precondition failure before execution.
	ErrPrecondition = 80
	// ErrResumable indicates the process exited early but can be resumed.
	ErrResumable = 90
)
