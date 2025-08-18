package device

import (
	"context"
	"os/exec"

	"lvmsync_go/internal/privilege"
)

type fakeEsc struct{ err error }

var _ privilege.Escalator = (*fakeEsc)(nil)

func (f fakeEsc) Ensure(context.Context) error { return f.err }

func (f fakeEsc) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
