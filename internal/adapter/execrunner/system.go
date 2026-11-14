package execrunner

import (
	"os"
	"os/user"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
)

// SystemClock is the wall clock.
type SystemClock struct{}

var _ execute.Clock = SystemClock{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }

// SystemUser names the invoking OS user from os/user, falling back to $USER.
type SystemUser struct {
	// Current looks up the user; nil means user.Current.
	Current func() (*user.User, error)
	// Getenv reads the environment; nil means os.Getenv.
	Getenv func(string) string
}

var _ execute.UserInfo = SystemUser{}

// Username returns the user name, $USER when the lookup fails, or "unknown".
func (s SystemUser) Username() string {
	current, getenv := s.Current, s.Getenv
	if current == nil {
		current = user.Current
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	if u, err := current(); err == nil && u.Username != "" {
		return u.Username
	}
	if u := getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}
