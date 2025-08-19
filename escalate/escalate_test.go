package escalate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	if strings.Contains(joined, "LD_PRELOAD=") || strings.Contains(joined, "GCONV_PATH=") {
		t.Fatalf("disallowed vars leaked: %v", out)
	}
	if strings.Contains(joined, "PATH=") || strings.Contains(joined, "LANG=") {
		t.Fatalf("PATH/LANG should be stripped: %v", out)
	}
	if len(out) != 1 || out[0] != "TERM=xterm-256color" {
		t.Fatalf("unexpected sanitized env: %v", out)
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

func TestIsRoot_RealGeteuid(t *testing.T) {
	want := os.Geteuid() == 0
	if got := IsRoot(Options{}); got != want {
		t.Fatalf("IsRoot() = %v, want %v", got, want)
	}
}

func TestIsRoot_MockedGeteuid(t *testing.T) {
	if !IsRoot(Options{Geteuid: func() int { return 0 }}) {
		t.Fatal("expected root")
	}
	if IsRoot(Options{Geteuid: func() int { return 1000 }}) {
		t.Fatal("expected non-root")
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
	m := &mockSys{}
	os.Unsetenv("SUDO_UID")
	os.Unsetenv("SUDO_GID")

	if err := DropToInvokerIfSudo(Options{Sys: m}, zap.NewNop()); err != nil {
		t.Fatalf("expected no-op nil err, got %v", err)
	}
}

func TestDropToInvokerIfSudo_Success(t *testing.T) {
	m := &mockSys{}
	t.Setenv("SUDO_UID", "1000")
	t.Setenv("SUDO_GID", "100")

	if err := DropToInvokerIfSudo(Options{Sys: m}, zap.NewNop()); err != nil {
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
	t.Setenv("SUDO_UID", "notnum")
	t.Setenv("SUDO_GID", "100")
	if err := DropToInvokerIfSudo(Options{Sys: m}, zap.NewNop()); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestDropToInvokerIfSudo_Logs(t *testing.T) {
	m := &mockSys{}
	t.Setenv("SUDO_UID", "1000")
	t.Setenv("SUDO_GID", "100")
	t.Setenv("LVMSYNC_ACTION_ID", "act123")
	argv := []string{"prog"}
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	if err := DropToInvokerIfSudo(Options{Sys: m, Args: argv}, logger); err != nil {
		t.Fatalf("DropToInvokerIfSudo error: %v", err)
	}
	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	host, _ := os.Hostname()
	first := entries[0].ContextMap()
	if first["action_id"] != "act123" {
		t.Fatalf("action_id = %v", first["action_id"])
	}
	if fmt.Sprint(first["argv"]) != fmt.Sprint(argv) {
		t.Fatalf("argv = %v", first["argv"])
	}
	if first["hostname"] != host {
		t.Fatalf("hostname = %v", first["hostname"])
	}
	second := entries[1].ContextMap()
	if second["result"] != "dropped" {
		t.Fatalf("result = %v", second["result"])
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
	}, zap.NewNop())
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
			return []string{"LD_PRELOAD=/x", "PATH=/usr/local/bin", "LANG=C", "TERM=dumb", "FOO=BAR"}
		},
		ExecRunner: fakeRunner(&got, nil),
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}
	reexeced, err := EnsureRootOrReexec(opts, zap.NewNop())
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
	if strings.Contains(envJoined, "LD_PRELOAD=") || strings.Contains(envJoined, "PATH=") || strings.Contains(envJoined, "LANG=") {
		t.Fatalf("env not sanitized: %v", got.env)
	}
	if len(got.env) != 1 || got.env[0] != "TERM=dumb" {
		t.Fatalf("unexpected env: %v", got.env)
	}
}

func TestEnsureRootOrReexec_SudoNotFound(t *testing.T) {
	_, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1000 },
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
	}, zap.NewNop())
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
	}, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "sudo escalation failed") {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
}

func TestEnsureRootOrReexec_RunnerExitCode(t *testing.T) {
	_, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1000 },
		LookPath: func(string) (string, error) { return "/usr/bin/sudo", nil },
		ExecRunner: func(string, []string, []string, io.Reader, io.Writer, io.Writer) error {
			return exec.Command("sh", "-c", "exit 42").Run()
		},
	}, zap.NewNop())
	var ee *exec.ExitError
	if err == nil || !errors.As(err, &ee) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if code := ee.ExitCode(); code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
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
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	joined := strings.Join(got.args, " ")
	if strings.Contains(joined, "--unsafe=1") {
		t.Fatalf("disallowed flag forwarded: %v", got.args)
	}
}

func TestEnsureRootOrReexec_InvokesSudoWithAllowlistedArgs(t *testing.T) {
	var got execCall
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:            func() int { return 1000 },
		LookPath:           func(string) (string, error) { return "/usr/bin/sudo", nil },
		Args:               []string{"/bin/prog", "--mode=apply", "--drop-back=false", "--unsafe=1"},
		AllowedPassthrough: map[string]bool{"--mode": true, "--drop-back": true},
		ExecRunner:         fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	if got.name != "/usr/bin/sudo" {
		t.Fatalf("expected sudo path, got %q", got.name)
	}
	if len(got.args) < 3 || got.args[0] != "-n" || got.args[1] != "--" {
		t.Fatalf("args malformed: %v", got.args)
	}
	joined := strings.Join(got.args, " ")
	if strings.Contains(joined, "--unsafe=1") {
		t.Fatalf("disallowed arg forwarded: %v", got.args)
	}
	if !strings.Contains(joined, "--mode=apply") || !strings.Contains(joined, "--drop-back=false") {
		t.Fatalf("allowed args missing: %v", got.args)
	}
}

func TestEnsureRootOrReexec_SanitizedEnv(t *testing.T) {
	var got execCall
	t.Setenv("LD_PRELOAD", "/tmp/x.so")
	t.Setenv("PATH", "/usr/local/bin")
	t.Setenv("LANG", "C")
	t.Setenv("TERM", "dumb")
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:     func() int { return 1000 },
		LookPath:    func(string) (string, error) { return "/usr/bin/sudo", nil },
		SanitizeEnv: true,
		ExecRunner:  fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	if got.env == nil {
		t.Fatal("expected sanitized env")
	}
	joined := strings.Join(got.env, "\n")
	if strings.Contains(joined, "LD_PRELOAD=") || strings.Contains(joined, "PATH=") || strings.Contains(joined, "LANG=") {
		t.Fatalf("disallowed vars present: %v", got.env)
	}
	if !strings.Contains(joined, "TERM=dumb") {
		t.Fatalf("TERM missing: %v", got.env)
	}
}

func TestEnsureRootOrReexec_DefaultUnsanitizedEnv(t *testing.T) {
	var got execCall
	t.Setenv("LD_PRELOAD", "/tmp/x.so")
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:    func() int { return 1000 },
		LookPath:   func(string) (string, error) { return "/usr/bin/sudo", nil },
		ExecRunner: fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	if len(got.env) != 0 {
		t.Fatalf("expected empty env to inherit unsanitized environment, got %v", got.env)
	}
}

func TestEnsureRootOrReexec_EnvPassthrough(t *testing.T) {
	var got execCall
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1000 },
		LookPath: func(string) (string, error) { return "/usr/bin/sudo", nil },
		Environ: func() []string {
			return []string{"PATH=/usr/local/bin", "LANG=C"}
		},
		ExecRunner: fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	if len(got.env) != 2 || got.env[0] != "PATH=/usr/local/bin" || got.env[1] != "LANG=C" {
		t.Fatalf("env not forwarded: %v", got.env)
	}
}

func TestEnsureRootOrReexec_SanitizeEnvTrue(t *testing.T) {
	var got execCall
	env := []string{"LD_PRELOAD=/tmp/x.so", "PATH=/usr/local/bin", "LANG=C", "TERM=dumb"}
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:     func() int { return 1000 },
		LookPath:    func(string) (string, error) { return "/usr/bin/sudo", nil },
		SanitizeEnv: true,
		Environ:     func() []string { return env },
		ExecRunner:  fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	want := sanitizedChildEnv(env)
	if !reflect.DeepEqual(got.env, want) {
		t.Fatalf("env = %v, want %v", got.env, want)
	}
}

func TestEnsureRootOrReexec_SanitizeEnvFalse(t *testing.T) {
	var got execCall
	env := []string{"PATH=/usr/local/bin", "LANG=C"}
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:     func() int { return 1000 },
		LookPath:    func(string) (string, error) { return "/usr/bin/sudo", nil },
		SanitizeEnv: false,
		Environ:     func() []string { return env },
		ExecRunner:  fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	if !reflect.DeepEqual(got.env, env) {
		t.Fatalf("env = %v, want %v", got.env, env)
	}
}

func TestEnsureRootOrReexec_SanitizeEnvStripsPathLang(t *testing.T) {
	var got execCall
	env := []string{"PATH=/usr/local/bin", "LANG=C"}
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:     func() int { return 1000 },
		LookPath:    func(string) (string, error) { return "/usr/bin/sudo", nil },
		SanitizeEnv: true,
		Environ:     func() []string { return env },
		ExecRunner:  fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	if len(got.env) != 0 {
		t.Fatalf("expected PATH/LANG stripped, got %v", got.env)
	}
}

func TestEnsureRootOrReexec_DefaultPreservesDisallowedEnv(t *testing.T) {
	var got execCall
	env := []string{"LD_PRELOAD=/tmp/x.so", "GCONV_PATH=/bad", "PATH=/usr/local/bin", "LANG=C"}
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:    func() int { return 1000 },
		LookPath:   func(string) (string, error) { return "/usr/bin/sudo", nil },
		Environ:    func() []string { return env },
		ExecRunner: fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	if !reflect.DeepEqual(got.env, env) {
		t.Fatalf("env = %v, want %v", got.env, env)
	}
}

func TestEnsureRootOrReexec_SanitizeEnvDropsDisallowed(t *testing.T) {
	var got execCall
	env := []string{"LD_PRELOAD=/tmp/x.so", "GCONV_PATH=/bad", "PATH=/usr/local/bin", "LANG=C", "TERM=dumb"}
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:     func() int { return 1000 },
		LookPath:    func(string) (string, error) { return "/usr/bin/sudo", nil },
		SanitizeEnv: true,
		Environ:     func() []string { return env },
		ExecRunner:  fakeRunner(&got, nil),
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	joined := strings.Join(got.env, "\n")
	if strings.Contains(joined, "LD_PRELOAD=") || strings.Contains(joined, "GCONV_PATH=") || strings.Contains(joined, "PATH=") || strings.Contains(joined, "LANG=") {
		t.Fatalf("disallowed vars present: %v", got.env)
	}
	if !strings.Contains(joined, "TERM=dumb") {
		t.Fatalf("TERM missing: %v", got.env)
	}
}

func TestEnsureRootOrReexec_DefaultRunnerPreservesEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.txt")
	script := filepath.Join(dir, "sudo.sh")
	scriptContent := fmt.Sprintf("#!/bin/sh\n/bin/cat /proc/self/environ > %s\n", envFile)
	if err := os.WriteFile(script, []byte(scriptContent), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("LD_PRELOAD", "/tmp/x.so")
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:  func() int { return 1000 },
		LookPath: func(string) (string, error) { return script, nil },
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	if !bytes.Contains(data, []byte("LD_PRELOAD=/tmp/x.so")) {
		t.Fatalf("environment sanitized unexpectedly: %q", data)
	}
}

func TestEnsureRootOrReexec_DefaultRunnerSanitizeEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.txt")
	script := filepath.Join(dir, "sudo.sh")
	scriptContent := fmt.Sprintf("#!/bin/sh\n/bin/cat /proc/self/environ > %s\n", envFile)
	if err := os.WriteFile(script, []byte(scriptContent), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("LD_PRELOAD", "/tmp/x.so")
	t.Setenv("TERM", "dumb")
	t.Setenv("PATH", "/usr/local/bin")
	reexeced, err := EnsureRootOrReexec(Options{
		Geteuid:     func() int { return 1000 },
		LookPath:    func(string) (string, error) { return script, nil },
		SanitizeEnv: true,
	}, zap.NewNop())
	if err != nil || !reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	if bytes.Contains(data, []byte("LD_PRELOAD=")) || bytes.Contains(data, []byte("PATH=")) {
		t.Fatalf("environment not sanitized: %q", data)
	}
	if !bytes.Contains(data, []byte("TERM=dumb")) {
		t.Fatalf("TERM missing: %q", data)
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
	}, zap.NewNop())
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

func TestEnsureRootOrReexec_Logs(t *testing.T) {
	t.Setenv("LVMSYNC_ACTION_ID", "act123")
	argv := []string{"prog"}
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	reexeced, err := EnsureRootOrReexec(Options{Args: argv, Geteuid: func() int { return 0 }}, logger)
	if err != nil || reexeced {
		t.Fatalf("unexpected result: %v %v", reexeced, err)
	}
	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	host, _ := os.Hostname()
	first := entries[0].ContextMap()
	if first["action_id"] != "act123" {
		t.Fatalf("action_id = %v", first["action_id"])
	}
	if fmt.Sprint(first["argv"]) != fmt.Sprint(argv) {
		t.Fatalf("argv = %v", first["argv"])
	}
	if first["hostname"] != host {
		t.Fatalf("hostname = %v", first["hostname"])
	}
	second := entries[1].ContextMap()
	if second["result"] != "already_root" {
		t.Fatalf("result = %v", second["result"])
	}
}
