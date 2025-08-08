package transfer

import (
	"os"
	"testing"
)

// newTempFile creates a temporary file and fails the test on error.
func newTempFile(t *testing.T, pattern string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), pattern)
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	return f
}
