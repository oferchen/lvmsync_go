// Package escalate implements a minimal, audited pattern to:
//  1. Detect whether the process is already privileged (EUID==0).
//  2. If not, re-exec itself via `sudo -n` with a tight argument allowlist
//     and a hardened environment.
//  3. Optionally drop back to the invoking user (SUDO_UID/GID) after doing
//     the privileged section.
//
// Design goals:
//   - Efficiency & correctness: zero reflection, zero global env mutation,
//     no deprecated packages (uses x/sys/unix), strict allowlist behavior.
//   - Testability: all external effects (geteuid, lookpath, exec) are overridable
//     via Options; syscalls for dropping privileges are abstracted behind a tiny
//     interface and are mocked in tests.
//   - Maintainability: tiny API surface, clean separation between pure helpers
//     and effectful code paths.
//
// Typical usage:
//
//			reexeced, err := escalate.EnsureRootOrReexec(escalate.Options{})
//			if err != nil { log.Fatal(err) }
//			if reexeced { return } // parent should exit after delegating to sudo
//
//			// ... privileged work ...
//	             if err := escalate.DropToInvokerIfSudo(escalate.Options{}); err != nil {
//	                     log.Fatal(err)
//	             }
//	             // ... continue unprivileged work ...
package escalate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Options control behavior and enable dependency injection for tests.
type Options struct {
	// AllowedPassthrough is a whitelist of flags (by key) to forward to the
	// sudo-reexeced process. Example keys: "--mode", "-m".
	AllowedPassthrough map[string]bool
	// ExtraArgs, if set, are appended after the self path (e.g., a subcommand).
	ExtraArgs []string
	// SanitizeEnv (default false) uses a hardened child environment.
	SanitizeEnv bool

	// Dependency seams (optional; default to real OS functions):
	Args       []string                          // defaults to os.Args
	Environ    func() []string                   // defaults to os.Environ
	Geteuid    func() int                        // defaults to os.Geteuid
	LookPath   func(file string) (string, error) // defaults to exec.LookPath
	ExecRunner func(name string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error
	Stdin      io.Reader     // defaults to nil (no prompting)
	Stdout     io.Writer     // defaults to os.Stdout
	Stderr     io.Writer     // defaults to os.Stderr
	Sys        syscallFacade // defaults to real unix syscalls
}

// IsRoot reports whether effective UID is 0 (overridable for tests via Options).
func IsRoot() bool { return os.Geteuid() == 0 }

// EnsureRootOrReexec ensures the process runs as root.
// If already root, returns (false, nil).
// If not root, re-execs the current binary through `sudo -n` and returns (true, err).
// When (true, nil) is returned, the caller should exit immediately.
func EnsureRootOrReexec(opts Options) (bool, error) {
	geteuid := opts.Geteuid
	if geteuid == nil {
		geteuid = os.Geteuid
	}
	if geteuid() == 0 {
		return false, nil
	}

	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	sudoPath, err := lookPath("sudo")
	if err != nil {
		return false, errors.New("sudo not found in PATH (install sudo or run as root)")
	}

	self, err := selfPath()
	if err != nil {
		return false, fmt.Errorf("resolve self path: %w", err)
	}

	argv := opts.Args
	if argv == nil {
		argv = os.Args
	}
	args := make([]string, 0, 3+len(opts.ExtraArgs)+len(argv))
	args = append(args, "-n", "--", self)
	if len(opts.ExtraArgs) > 0 {
		args = append(args, opts.ExtraArgs...)
	}
	args = append(args, filterAllowed(argv[1:], opts.AllowedPassthrough)...)

	var env []string
	switch {
	case opts.SanitizeEnv:
		if opts.Environ != nil {
			env = sanitizedChildEnv(opts.Environ())
		} else {
			env = sanitizedChildEnv(os.Environ())
		}
	case opts.Environ != nil:
		env = opts.Environ()
	}

	stdin := opts.Stdin // nil by default (no TTY password prompts)
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	run := opts.ExecRunner
	if run == nil {
		run = defaultExecRunner
	}

	if err := run(sudoPath, args, env, stdin, stdout, stderr); err != nil {
		return false, fmt.Errorf("sudo escalation failed: %w", err)
	}
	return true, nil
}

// DropToInvokerIfSudo drops privileges back to the invoking user if launched
// via sudo (SUDO_UID/SUDO_GID). No-op if those env vars are absent.
// Dependency seams are provided via opts.
func DropToInvokerIfSudo(opts Options) error {
	suid := os.Getenv("SUDO_UID")
	sgid := os.Getenv("SUDO_GID")
	if suid == "" || sgid == "" {
		return nil
	}
	uid, err := parseInt(suid)
	if err != nil {
		return fmt.Errorf("parse SUDO_UID (%q): %w", suid, err)
	}
	gid, err := parseInt(sgid)
	if err != nil {
		return fmt.Errorf("parse SUDO_GID (%q): %w", sgid, err)
	}
	sys := opts.Sys
	if sys == nil {
		sys = unixFacade{}
	}
	if err := sys.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := sys.Setresgid(gid, gid, gid); err != nil {
		return fmt.Errorf("setresgid: %w", err)
	}
	if err := sys.Setresuid(uid, uid, uid); err != nil {
		return fmt.Errorf("setresuid: %w", err)
	}
	return nil
}

// ---- Internal helpers (kept small, pure, and fully tested) ----

func sanitizedChildEnv(environ []string) []string {
	const safePath = "PATH=/usr/sbin:/usr/bin:/sbin:/bin"
	whitelist := map[string]bool{
		"LANG":     true,
		"LC_ALL":   true,
		"LC_CTYPE": true,
		"TERM":     true,
	}
	out := make([]string, 0, 1+len(environ))
	out = append(out, safePath)
	for _, kv := range environ {
		if strings.HasPrefix(kv, "LD_") || strings.HasPrefix(kv, "GCONV_PATH=") {
			continue
		}
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		k := kv[:i]
		if whitelist[k] {
			out = append(out, kv)
		}
	}
	return out
}

func filterAllowed(argv []string, allow map[string]bool) []string {
	if len(argv) == 0 || len(allow) == 0 {
		return nil
	}
	out := make([]string, 0, len(argv))
	for _, a := range argv {
		k := a
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			k = a[:eq]
		}
		if allow[k] {
			out = append(out, a)
		}
	}
	return out
}

func parseInt(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid digit %q", c)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func selfPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		// fallback to argv[0] for completeness
		return filepath.Abs(os.Args[0])
	}
	if !strings.HasPrefix(p, "/") {
		return filepath.Abs(p)
	}
	return p, nil
}

// ---- System seams ----

type syscallFacade interface {
	Setgroups([]int) error
	Setresgid(int, int, int) error
	Setresuid(int, int, int) error
}

type unixFacade struct{}

func (unixFacade) Setgroups(gids []int) error  { return unix.Setgroups(gids) }
func (unixFacade) Setresgid(r, e, s int) error { return unix.Setresgid(r, e, s) }
func (unixFacade) Setresuid(r, e, s int) error { return unix.Setresuid(r, e, s) }

func defaultExecRunner(name string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
