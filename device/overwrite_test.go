package device

import (
	"context"
	"io"
	"strings"
	"testing"
)

// overwriteTTYReader wraps an io.Reader and implements Fd() to simulate a TTY.
type overwriteTTYReader struct{ io.Reader }

func (overwriteTTYReader) Fd() uintptr { return 0 }

func TestConfirmOverwriteMixedCase(t *testing.T) {
	ctx := WithForce(context.Background(), true)
	r := overwriteTTYReader{strings.NewReader("YeS\n")}
	if err := confirmOverwrite(ctx, r, io.Discard, func(int) bool { return true }); err != nil {
		t.Fatalf("confirmOverwrite: %v", err)
	}
}
