package lvmsync

// Version information populated at build time via -ldflags.
var (
	// Version is the semantic version of the binary.
	Version = "dev"
	// Commit is the git commit hash for this build.
	Commit = "unknown"
	// Date is the build timestamp in RFC3339 format.
	Date = "unknown"
)

func init() {
	_ = Version
	_ = Commit
	_ = Date
}
