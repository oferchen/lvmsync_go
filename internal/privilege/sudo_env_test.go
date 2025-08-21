package privilege

import (
        "context"
        "os/exec"
        "strings"
        "testing"

        "go.uber.org/zap"
)

// dummy commander to avoid executing real commands
func noopCmd(ctx context.Context, _ string, _ ...string) *exec.Cmd {
       return exec.CommandContext(ctx, "echo")
}

func TestSanitizeEnv(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"LANG=en_US.UTF-8",
		"LC_ALL=C",
		"TERM=xterm",
		"LD_PRELOAD=evil.so",
		"GCONV_PATH=/tmp",
		"FOO=bar",
	}
	got := sanitizeEnv(env)
	for _, bad := range []string{"PATH=", "LANG=", "LD_PRELOAD=", "GCONV_PATH=", "FOO="} {
		for _, kv := range got {
			if strings.HasPrefix(kv, bad) {
				t.Fatalf("unexpected variable %s in sanitized env", bad)
			}
		}
	}
	want := map[string]bool{"LC_ALL=C": true, "TERM=xterm": true}
	for _, kv := range got {
		if !want[kv] {
			t.Fatalf("unexpected variable %s", kv)
		}
		delete(want, kv)
	}
	if len(want) != 0 {
		t.Fatalf("missing variables %v", want)
	}
}

func TestCommandAppliesSanitizedEnv(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "C")
	t.Setenv("TERM", "xterm")
	t.Setenv("LD_PRELOAD", "evil.so")
        esc := &sudoEscalator{sanitizeEnv: true, runner: &Runner{Cmd: commanderFunc(noopCmd), Logger: zap.NewNop()}}
	cmd := esc.Command(context.Background(), "true")
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "LANG=") || strings.HasPrefix(kv, "LD_PRELOAD=") {
			t.Fatalf("unsanitized variable %s", kv)
		}
	}
	if !contains(cmd.Env, "LC_ALL=C") || !contains(cmd.Env, "TERM=xterm") {
		t.Fatalf("expected LC_ALL and TERM in env: %v", cmd.Env)
	}
}

func contains(env []string, v string) bool {
	for _, e := range env {
		if e == v {
			return true
		}
	}
	return false
}
