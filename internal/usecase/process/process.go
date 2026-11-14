// Package process holds the value types of the CommandRunner port: the
// subprocess to run (argv only, never a shell string) and its result. Each
// use case that runs commands declares its own CommandRunner interface over
// these types, so one runner adapter satisfies all of them.
package process

import (
	"errors"
	"io"
	"time"
)

// Errors a runner wraps.
var (
	// ErrTimeout means the command exceeded Cmd.Timeout and was killed.
	ErrTimeout = errors.New("command timed out")
	// ErrNotFound means the program could not be found or started.
	ErrNotFound = errors.New("program not found")
)

// Cmd is one subprocess invocation.
type Cmd struct {
	// Argv is the exact argv; Argv[0] is the program (looked up on the
	// child PATH when it has no slash).
	Argv []string
	// Env overrides the runner's base environment for this command. The
	// runner always removes HOMEBREW_NO_REQUIRE_TAP_TRUST* from the result.
	Env map[string]string
	// Dir is the working directory; empty means the runner's.
	Dir string
	// Interactive hands the parent's terminal (stdin, stdout, stderr) to the
	// child, for sudo prompts. Output is then not captured.
	Interactive bool
	// Log receives a header (argv and env variable names, never values) and
	// a copy of the output. Nil means no log.
	Log io.Writer
	// Timeout kills the child after this long; zero means no timeout.
	Timeout time.Duration
}

// Result is how a command ended. A non-zero exit is a Result, not an error.
type Result struct {
	// ExitCode is the child's exit code.
	ExitCode int
	// Stdout is the captured standard output (empty when Interactive).
	Stdout string
	// Stderr is the captured standard error (empty when Interactive).
	Stderr string
}

// Success reports whether the command exited 0.
func (r Result) Success() bool { return r.ExitCode == 0 }
