package device

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// confirmOverwrite ensures destructive operations are allowed. It requires
// --force and either typing the literal text "double-confirm" on a TTY or the
// combination of --allow-overwrite and --yes-i-know. stdin and stderr are used
// for prompting, and isTerminal reports whether the stdin file descriptor is a
// TTY.
func confirmOverwrite(ctx context.Context, stdin io.Reader, stderr io.Writer, isTerminal func(int) bool) error {
	if !forceFromContext(ctx) {
		return fmt.Errorf("--force required for write operations")
	}
	allow := allowOverwriteFromContext(ctx)
	yesIKnow := yesIKnowFromContext(ctx)
	if allow && yesIKnow {
		return nil
	}
	type fdProvider interface{ Fd() uintptr }
	if f, ok := stdin.(fdProvider); ok && isTerminal(int(f.Fd())) {
		fmt.Fprint(stderr, "Device operations may overwrite data. Type 'double-confirm' to continue: ")
		reader := bufio.NewReader(stdin)
		resp, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if strings.TrimSpace(resp) != "double-confirm" {
			return fmt.Errorf("operation cancelled")
		}
		return nil
	}
	if !allow {
		return fmt.Errorf("--allow-overwrite required for non-interactive write operations")
	}
	return fmt.Errorf("--yes-i-know required for non-interactive write operations")
}
