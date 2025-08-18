package escalate

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// --- Pure helper tests ---

func TestFilterAllowed(t *testing.T) {
	argv := []string{"--mode=apply", "--unsafe=1", "-v", "--drop-back=false"}
	allow := map[string]bool{"--mode": true, "--drop-back": true}
	got := filterAllowed(argv, allow)
	want := []string{"--mode=apply", "--drop-back=false"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("filterAllowed = %v, want %v", got, want)
	}
}

func TestSanitizedChildEnv(t *testing.T) {
	in := []string{
		"LD_PRELOAD=/tmp/x.so",
		"GCONV_PATH=/bad",
		"PATH=/usr/local/bin",
		"LANG=C",
		"FOO=BAR",
		"TERM=xterm-256color",
	}
	out := sanitizedChildEnv(in)
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "LD_PRELOAD=") {
		t.Fatalf("LD_* leaked into env: %v", out)
	}
	if !strings.Contains(joined, "LANG=C") || !strings.Contains(joined, "TERM=xterm-256color") {
		t.Fatalf("whitelisted vars missing: %v", out)
	}
	if !strings.HasPrefix(out[0], "PATH=") {
		t.Fatalf("first entry must be safe PATH, got %q", out[0])
	}
}

func TestParseInt(t *testing.T) {
	n, err := parseInt("12345")
	if err != nil || n != 12345 {
		t.Fatalf("parseInt ok path: got (%d,%v)", n, err)
	}
	if _, err = parseInt(""); err == nil {
		t.Fatal("parseInt empty should error")
	}
	if _, err = parseInt("1a"); err == nil {
		t.Fatal("parseInt invalid digit should error")
	}
}

func TestSelfPath(t *testing.T) {
	p, err := selfPath()
	if err != nil || p == "" {
		t.Fatalf("selfPath failed: %v %q", err, p)
	}
}

// --- DropToInvokerIfSudo tests (mock syscalls) ---

type mockSys struct {
	groups []int
	setG   [3]int
	setU   [3]int
	errG   error
	errSG  error
	errSU  error
}

func (m *mockSys) Setgroups(gids []int) error {
	m.groups = gids
	return m.errG
}
func (m *mockSys) Setresgid(r, e, s int) error {
	m.setG = [3]int{r, e, s}
	return m.errSG
}
func (m *mockSys) Setresuid(r, e, s int) error {
	m.setU = [3]int{r, e, s}
	return m.errSU
}

func TestDropToInvokerIfSudo_NoEnv(t *testing.T) {
	old := sys
	sys = &mockSys{}
	t.Cleanup(func() { sys = old })
	os.Unsetenv("SUDO_UID")
	os.Unsetenv("SUDO_GID")

	if err := DropToInvokerIfSudo(); err != nil {
		t.Fatalf("expected no-op nil err, got %v", err)
	}
}

func TestDropToInvokerIfSudo_Success(t *testing.T) {
	m := &mockSys{}
	old := sys
	sys = m
	t.Cleanup(func() { sys = old })

	t.Setenv("SUDO_UID", "1000")
	t.Setenv("SUDO_GID", "100")

	if err := DropToInvokerIfSudo(); err != nil {
		t.Fatalf("DropToInvokerIfSudo error: %v", err)
	}
	if len(m.groups) != 1 || m.groups[0] != 100 {
		t.Fatalf("Setgroups got %v", m.groups)
	}
	if m.setG != [3]int{100, 100, 100} {
		t.Fatalf("Setresgid got %v", m.setG)
	}
	if m.setU != [3]int{1000, 1000, 1000} {
		t.Fatalf("Setresuid got %v", m.setU)
	}
}

func TestDropToInvokerIfSudo_ParseError(t *testing.T) {
	m := &mockSys{}
	old := sys
	sys = m
	t.Cleanup(func() { sys = old })

	t.Setenv("SUDO_UID", "notnum")
	t.Setenv("SUDO_GID", "100")
	if err := DropToInvokerIfSudo(); err == nil {
		t.Fatal("expected parse error")
	}
}

// --- EnsureRootOrReexec tests (mock exec & lookpath & geteuid) ---

type execCall struct {
	name string
	args []string
	env  []string
}

func fakeRunner(rec *execCall, ret error) func(string, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return func(name string, args []string, env []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
		*rec = execCall{name: name, args: append([]string{}, args...), env: append([]string{}, env...)}
		return ret
	}
}

func TestEnsureRootOrReexec_AlreadyRoot(t *testing.T) {
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid: func() int { return 0 },
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if reexeced {
		t.Fatal("reexeced should be false for root")
	}
}

func TestEnsureRootOrReexec_ReexecHappyPath(t *testing.T) {
	var got execCall
	opts := Options{
		Geteuid: func() int { return 1000 },
		LookPath: func(s string) (string, error) {
			if s == "sudo" {
				return "/usr/bin/sudo", nil
			}
			return "", errors.New("not found")
		},
		Args:               []string{"/bin/prog", "--mode=apply", "--drop-back=false", "--unsafe=1"},
		AllowedPassthrough: map[string]bool{"--mode": true, "--drop-back": true},
		ExtraArgs:          []string{"--do-privileged"},
		SanitizeEnv:        true,
		Environ: func() []string {
			return []string{"LD_PRELOAD=/x", "LANG=C", "TERM=dumb", "FOO=BAR"}
		},
		ExecRunner: fakeRunner(&got, nil),
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}
	reexeced, err := EnsureRootOrReexec(opts)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !reexeced {
		t.Fatal("expected reexeced == true")
	}
	if got.name != "/usr/bin/sudo" {
		t.Fatalf("wrong runner name: %q", got.name)
	}
	if len(got.args) < 4 || got.args[0] != "-n" || got.args[1] != "--" || got.args[3] != "--do-privileged" {
		t.Fatalf("args malformed: %v", got.args)
	}
	joined := strings.Join(got.args, " ")
	if !strings.Contains(joined, "--mode=apply") || !strings.Contains(joined, "--drop-back=false") {
		t.Fatalf("allowed flags not forwarded: %v", got.args)
	}
	envJoined := strings.Join(got.env, "\n")
	if strings.Contains(envJoined, "LD_PRELOAD=") || !strings.Contains(envJoined, "LANG=") {
		t.Fatalf("env not sanitized: %v", got.env)
	}
}

func TestEnsureRootOrReexec_SudoNotFound(t *testing.T) {
	_, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1000 },
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
	})
	if err == nil || !strings.Contains(err.Error(), "sudo not found") {
		t.Fatalf("expected sudo not found error, got %v", err)
	}
}

func TestEnsureRootOrReexec_RunnerError(t *testing.T) {
	var got execCall
	_, err := EnsureRootOrReexec(Options{
		Geteuid:    func() int { return 1000 },
		LookPath:   func(string) (string, error) { return "/usr/bin/sudo", nil },
		ExecRunner: fakeRunner(&got, errors.New("boom")),
	})
	if err == nil || !strings.Contains(err.Error(), "sudo escalation failed") {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
}

func TestEnsureRootOrReexec_DropsDisallowedFlags(t *testing.T) {
	var got execCall
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:            func() int { return 1000 },
		LookPath:           func(string) (string, error) { return "/usr/bin/sudo", nil },
		Args:               []string{"/bin/prog", "--mode=apply", "--unsafe=1"},
		AllowedPassthrough: map[string]bool{"--mode": true},
		ExecRunner:         fakeRunner(&got, nil),
	})
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	joined := strings.Join(got.args, " ")
	if strings.Contains(joined, "--unsafe=1") {
		t.Fatalf("disallowed flag forwarded: %v", got.args)
	}
}

func TestEnsureRootOrReexec_ErrorLogging(t *testing.T) {
	var got execCall
	core, logs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)
	t.Cleanup(func() { _ = logger.Sync() })

	_, err := EnsureRootOrReexec(Options{
		Geteuid:    func() int { return 1000 },
		LookPath:   func(string) (string, error) { return "/usr/bin/sudo", nil },
		ExecRunner: fakeRunner(&got, errors.New("boom")),
	})
	if err == nil {
		t.Fatal("expected error")
	}

	logger.Error("escalation_failed", zap.Error(err))
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Message != "escalation_failed" {
		t.Fatalf("wrong message: %q", e.Message)
	}
	if len(e.Context) == 0 || e.Context[0].Key != "error" {
		t.Fatalf("log missing error field: %+v", e.Context)
	}
	if errField, ok := e.Context[0].Interface.(error); !ok || errField.Error() != err.Error() {
		t.Fatalf("unexpected error value: %+v", e.Context[0])
	}
}
