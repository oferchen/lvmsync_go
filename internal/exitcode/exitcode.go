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
)
