// Package execrunner runs subprocesses for facman: argv only (never a
// shell), with the child environment built from a base plus per-command
// overrides and every HOMEBREW_NO_REQUIRE_TAP_TRUST* variable removed,
// optional timeouts, output teed to a log whose header lists environment
// variable names only, and interactive hand-off of the terminal for sudo
// prompts. It also provides the system Clock and UserInfo.
package execrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/bootstrap"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

// waitDelay bounds how long Run waits for output pipes after the child
// exits or is killed (a grandchild may hold them open).
const waitDelay = 2 * time.Second

// Options configure a Runner.
type Options struct {
	// BaseEnv is the base environment as KEY=VALUE pairs (usually os.Environ()).
	BaseEnv []string
	// Stdin, Stdout and Stderr are the parent's terminal, given to
	// interactive commands. Nil means os.Stdin, os.Stdout, os.Stderr.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Runner runs commands. It is safe for concurrent use.
type Runner struct {
	base   map[string]string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

var (
	_ bootstrap.CommandRunner = (*Runner)(nil)
	_ scan.CommandRunner      = (*Runner)(nil)
	_ execute.CommandRunner   = (*Runner)(nil)
	_ bootstrap.PathLookup    = (*Runner)(nil)
)

// New returns a Runner.
func New(opts Options) *Runner {
	r := &Runner{base: parseEnv(opts.BaseEnv), stdin: opts.Stdin, stdout: opts.Stdout, stderr: opts.Stderr}
	if r.stdin == nil {
		r.stdin = os.Stdin
	}
	if r.stdout == nil {
		r.stdout = os.Stdout
	}
	if r.stderr == nil {
		r.stderr = os.Stderr
	}
	return r
}

func parseEnv(kvs []string) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			m[k] = v
		}
	}
	return m
}

// Environ returns the child environment for overrides: base + overrides,
// minus every HOMEBREW_NO_REQUIRE_TAP_TRUST* variable, sorted by name.
func (r *Runner) Environ(overrides map[string]string) []string {
	merged := make(map[string]string, len(r.base)+len(overrides))
	for k, v := range r.base {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	out := make([]string, 0, len(merged))
	for k, v := range merged {
		if strings.HasPrefix(k, plan.TapTrustEnvPrefix) {
			continue
		}
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// LookPath finds name on the base PATH. A name with a slash is checked as is.
func (r *Runner) LookPath(name string) (string, error) {
	return lookPath(name, r.base["PATH"])
}

func lookPath(name, path string) (string, error) {
	if strings.Contains(name, "/") {
		if isExecutable(name) {
			return name, nil
		}
		return "", fmt.Errorf("%w: %s", process.ErrNotFound, name)
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if isExecutable(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: %s not on PATH", process.ErrNotFound, name)
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode().Perm()&0o111 != 0
}

// lockedWriter serializes writes from stdout and stderr copiers.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// Run runs cmd. Argv[0] is looked up on the child PATH. A non-zero exit is
// a Result, not an error. Errors wrap process.ErrNotFound (the program does
// not exist), process.ErrTimeout (Timeout elapsed; the child was killed) or
// the context error (ctx was cancelled; the child was killed). Callers that
// must let a command finish, such as installs, pass a context that is not
// cancelled.
func (r *Runner) Run(ctx context.Context, cmd process.Cmd) (process.Result, error) {
	if len(cmd.Argv) == 0 {
		return process.Result{}, fmt.Errorf("running command: %w: empty argv", process.ErrNotFound)
	}
	env := r.Environ(cmd.Env)
	childPath := r.base["PATH"]
	if p, ok := cmd.Env["PATH"]; ok {
		childPath = p
	}
	prog, err := lookPath(cmd.Argv[0], childPath)
	if err != nil {
		return process.Result{ExitCode: -1}, fmt.Errorf("running %s: %w", cmd.Argv[0], err)
	}
	log := io.Discard
	if cmd.Log != nil {
		log = &lockedWriter{w: cmd.Log}
		if err := writeHeader(log, cmd); err != nil {
			return process.Result{ExitCode: -1}, fmt.Errorf("writing log for %s: %w", cmd.Argv[0], err)
		}
	}

	runCtx := ctx
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}
	c := exec.CommandContext(runCtx, prog, cmd.Argv[1:]...) //nolint:gosec // argv comes from the reviewed plan or a catalog probe; never a shell
	c.Args[0] = cmd.Argv[0]
	c.Env = env
	c.Dir = cmd.Dir
	c.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	if cmd.Interactive {
		c.Stdin, c.Stdout, c.Stderr = r.stdin, r.stdout, r.stderr
	} else {
		c.Stdout = io.MultiWriter(&stdout, log)
		c.Stderr = io.MultiWriter(&stderr, log)
	}
	runErr := c.Run()
	res := process.Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if c.ProcessState != nil {
		res.ExitCode = c.ProcessState.ExitCode()
	}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case runCtx.Err() != nil && ctx.Err() == nil:
		res.ExitCode = -1
		return res, fmt.Errorf("running %s: %w after %s", cmd.Argv[0], process.ErrTimeout, cmd.Timeout)
	case ctx.Err() != nil:
		res.ExitCode = -1
		return res, fmt.Errorf("running %s: %w", cmd.Argv[0], ctx.Err())
	case errors.As(runErr, &exitErr):
		// a non-zero exit is reported in the result
	default:
		res.ExitCode = -1
		return res, fmt.Errorf("running %s: %w", cmd.Argv[0], runErr)
	}
	if cmd.Log != nil {
		if _, err := fmt.Fprintf(log, "# exit %d\n", res.ExitCode); err != nil {
			return res, fmt.Errorf("writing log for %s: %w", cmd.Argv[0], err)
		}
	}
	return res, nil
}

// writeHeader logs the argv and the names (never values) of the variables
// set for the command.
func writeHeader(w io.Writer, cmd process.Cmd) error {
	quoted := make([]string, len(cmd.Argv))
	for i, a := range cmd.Argv {
		quoted[i] = strconv.Quote(a)
	}
	names := make([]string, 0, len(cmd.Env))
	for k := range cmd.Env {
		if !strings.HasPrefix(k, plan.TapTrustEnvPrefix) {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	header := "$ " + strings.Join(quoted, " ") + "\n# env: " + strings.Join(names, " ") + "\n"
	if cmd.Interactive {
		header += "# interactive: output went to the terminal\n"
	}
	_, err := io.WriteString(w, header)
	return err
}
