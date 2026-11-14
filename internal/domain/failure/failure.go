// Package failure defines the typed errors that decide facman's exit codes
// (PRD Appendix H): usage 64, validation 3, bootstrap 4, operation 1, drift 2.
package failure

import (
	"errors"
	"fmt"
	"strings"
)

// Kind classifies a failure; each kind maps to one exit code.
type Kind string

// Failure kinds.
const (
	KindUsage      Kind = "usage"
	KindValidation Kind = "validation"
	KindBootstrap  Kind = "bootstrap"
	KindOperation  Kind = "operation"
	KindDrift      Kind = "drift"
)

// ExitCode returns the process exit code for the kind. Unknown kinds are 1.
func (k Kind) ExitCode() int {
	switch k {
	case KindUsage:
		return 64
	case KindValidation:
		return 3
	case KindBootstrap:
		return 4
	case KindDrift:
		return 2
	default:
		return 1
	}
}

// rank orders kinds for Worst: higher wins.
func (k Kind) rank() int {
	switch k {
	case KindUsage:
		return 5
	case KindBootstrap:
		return 4
	case KindValidation:
		return 3
	case KindDrift:
		return 1
	default: // operation and anything untyped
		return 2
	}
}

// Error is a failure with a kind, a message, an optional fix and cause.
type Error struct {
	// Kind decides the exit code.
	Kind Kind
	// Msg names the cause.
	Msg string
	// Fix tells the user what to do, if known.
	Fix string
	// Details lists individual problems (for example every catalog error).
	Details []string
	// Err is the wrapped cause, if any.
	Err error
}

// Error returns the message followed by any details.
func (e *Error) Error() string {
	msg := e.Msg
	if len(e.Details) > 0 {
		msg += ": " + strings.Join(e.Details, "; ")
	}
	if e.Err != nil && !strings.Contains(msg, e.Err.Error()) {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Err }

// ExitCode returns the exit code for the error's kind.
func (e *Error) ExitCode() int { return e.Kind.ExitCode() }

// WithFix returns a copy of e with Fix set.
func (e *Error) WithFix(fix string) *Error {
	c := *e
	c.Fix = fix
	return &c
}

// WithDetails returns a copy of e with Details set.
func (e *Error) WithDetails(details ...string) *Error {
	c := *e
	c.Details = append([]string(nil), details...)
	return &c
}

// Wrap returns a copy of e that wraps err.
func (e *Error) Wrap(err error) *Error {
	c := *e
	c.Err = err
	return &c
}

func newf(kind Kind, format string, args ...any) *Error {
	built := fmt.Errorf(format, args...)
	return &Error{Kind: kind, Msg: built.Error(), Err: errors.Unwrap(built)}
}

// Usage is a command-line usage error (exit 64). The format follows
// fmt.Errorf, so a %w verb records the cause.
func Usage(format string, args ...any) *Error { return newf(KindUsage, format, args...) }

// Validation is a catalog, profile or input validation error (exit 3).
func Validation(format string, args ...any) *Error { return newf(KindValidation, format, args...) }

// Bootstrap is a machine prerequisite or state error (exit 4).
func Bootstrap(format string, args ...any) *Error { return newf(KindBootstrap, format, args...) }

// Operation is a failed or skipped install (exit 1).
func Operation(format string, args ...any) *Error { return newf(KindOperation, format, args...) }

// Drift is a doctor finding (exit 2).
func Drift(format string, args ...any) *Error { return newf(KindDrift, format, args...) }

// KindOf returns the kind of the first *Error in err's chain.
func KindOf(err error) (Kind, bool) {
	var fe *Error
	if errors.As(err, &fe) {
		return fe.Kind, true
	}
	return "", false
}

// ExitCode maps any error to an exit code: nil is 0, an untyped error is 1.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if k, ok := KindOf(err); ok {
		return k.ExitCode()
	}
	return 1
}

// Worst returns the error that decides the exit code, by precedence
// usage > bootstrap > validation > operation > drift. Untyped errors count as
// operation errors. Ties keep the first. Nil errors are ignored.
func Worst(errs ...error) error {
	var worst error
	best := 0
	for _, err := range errs {
		if err == nil {
			continue
		}
		k, _ := KindOf(err)
		if r := k.rank(); r > best {
			worst, best = err, r
		}
	}
	return worst
}
