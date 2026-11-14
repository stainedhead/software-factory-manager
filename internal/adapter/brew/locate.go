// Package brew is the Homebrew adapter: it locates brew (PATH, then the
// known prefixes), evaluates `brew shellenv` without a shell, parses
// `brew info --json=v2`, `brew tap` and `brew trust` listings, detects
// installed formulae and casks with one read-only call, and implements the
// brew-formula and brew-cask install backends (exact argv; never --force,
// --overwrite or link; item-level tap trust only).
package brew

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/bootstrap"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// readTimeout bounds every read-only brew call (detector timeout, FR-010).
const readTimeout = 10 * time.Second

// Runner runs brew. Implemented by adapter/execrunner.
type Runner interface {
	Run(ctx context.Context, cmd process.Cmd) (process.Result, error)
}

// ReadOnlyEnv are the variables set on every read-only brew call so it
// makes no network calls (D-15, AD-13). Install calls do not get them.
func ReadOnlyEnv() map[string]string {
	return map[string]string{"HOMEBREW_NO_AUTO_UPDATE": "1", "HOMEBREW_NO_ENV_HINTS": "1"}
}

// DefaultPrefixes returns Homebrew's known prefixes for goos, in search order.
func DefaultPrefixes(goos string) []string {
	if goos == "darwin" {
		return []string{"/opt/homebrew", "/usr/local"}
	}
	return []string{"/home/linuxbrew/.linuxbrew"}
}

// LocatorOptions configure a Locator. Zero values take the real defaults.
type LocatorOptions struct {
	// KnownPrefixes are searched when brew is not on PATH; default
	// DefaultPrefixes(runtime.GOOS).
	KnownPrefixes []string
	// Getenv reads the base environment (PATH, and variables shellenv
	// output refers to); default os.Getenv.
	Getenv func(string) string
	// UID is the invoking user's UID; default os.Getuid().
	UID func() int
	// OwnerUID returns the owner UID of a path; default os.Stat.
	OwnerUID func(path string) (int, error)
}

// Locator implements bootstrap.HomebrewLocator.
type Locator struct {
	runner Runner
	opts   LocatorOptions
}

var _ bootstrap.HomebrewLocator = (*Locator)(nil)

// NewLocator returns a Locator that runs brew through runner.
func NewLocator(runner Runner, opts LocatorOptions) *Locator {
	if opts.KnownPrefixes == nil {
		opts.KnownPrefixes = DefaultPrefixes(runtime.GOOS)
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	if opts.UID == nil {
		opts.UID = os.Getuid
	}
	if opts.OwnerUID == nil {
		opts.OwnerUID = ownerUID
	}
	return &Locator{runner: runner, opts: opts}
}

func ownerUID(path string) (int, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("checking owner of %s: %w", path, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("checking owner of %s: no unix stat", path)
	}
	return int(st.Uid), nil
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode().Perm()&0o111 != 0
}

var versionRe = regexp.MustCompile(`Homebrew (\S+)`)

// Locate finds brew on PATH, then at the known prefixes. The prefix is the
// parent of brew's bin directory. It reports when the prefix belongs to
// another user, and brew's version ("" when it cannot be read).
func (l *Locator) Locate(ctx context.Context) (platform.HomebrewInfo, error) {
	var info platform.HomebrewInfo
	for _, dir := range filepath.SplitList(l.opts.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if p := filepath.Join(dir, "brew"); isExecutable(p) {
			info = platform.HomebrewInfo{Found: true, Path: p, Prefix: filepath.Dir(filepath.Clean(dir)), OnPATH: true}
			break
		}
	}
	if !info.Found {
		for _, prefix := range l.opts.KnownPrefixes {
			if p := filepath.Join(prefix, "bin", "brew"); isExecutable(p) {
				info = platform.HomebrewInfo{Found: true, Path: p, Prefix: prefix}
				break
			}
		}
	}
	if !info.Found {
		return info, nil
	}
	uid, err := l.opts.OwnerUID(info.Prefix)
	if err != nil {
		return info, fmt.Errorf("locating Homebrew: %w", err)
	}
	info.PrefixOwnedByOther = uid != l.opts.UID()
	res, err := l.runner.Run(ctx, process.Cmd{Argv: []string{info.Path, "--version"}, Env: ReadOnlyEnv(), Timeout: readTimeout})
	if err == nil && res.Success() {
		if m := versionRe.FindStringSubmatch(res.Stdout); m != nil {
			info.Version = m[1]
		}
	}
	return info, nil
}

// ShellEnv runs `brew shellenv bash` and parses its export statements into
// a map without running a shell. References to variables ($PATH,
// ${PATH+:$PATH}, ${INFOPATH:-}) are expanded from the base environment;
// statements it cannot read (conditionals) are skipped.
func (l *Locator) ShellEnv(ctx context.Context, brewPath string) (map[string]string, error) {
	res, err := l.runner.Run(ctx, process.Cmd{Argv: []string{brewPath, "shellenv", "bash"}, Env: ReadOnlyEnv(), Timeout: readTimeout})
	if err != nil {
		return nil, fmt.Errorf("running brew shellenv: %w", err)
	}
	if !res.Success() {
		return nil, fmt.Errorf("brew shellenv exited %d: %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	env := ParseShellEnv(res.Stdout, l.opts.Getenv)
	if len(env) == 0 {
		return nil, errors.New("brew shellenv printed no export statements")
	}
	return env, nil
}

// ParseShellEnv reads `export NAME="value";` statements. Variable
// references are expanded with getenv, or with values set earlier in the
// same output.
func ParseShellEnv(out string, getenv func(string) string) map[string]string {
	env := map[string]string{}
	lookup := func(name string) (string, bool) {
		if v, ok := env[name]; ok {
			return v, true
		}
		v := getenv(name)
		return v, v != ""
	}
	for _, stmt := range strings.FieldsFunc(out, func(r rune) bool { return r == '\n' || r == ';' }) {
		stmt = strings.TrimSpace(stmt)
		rest, ok := strings.CutPrefix(stmt, "export ")
		if !ok {
			continue
		}
		name, value, ok := strings.Cut(strings.TrimSpace(rest), "=")
		if !ok || !validName(name) {
			continue
		}
		value = unquote(value)
		expanded, ok := expand(value, lookup)
		if !ok {
			continue
		}
		env[name] = expanded
	}
	return env
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

// expand handles $NAME, ${NAME}, ${NAME+word}, ${NAME-word}, ${NAME:-word}
// and ${NAME#prefix}. Anything else (command substitution) is unreadable.
func expand(s string, lookup func(string) (string, bool)) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '`':
			return "", false
		case c != '$':
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		if s[i+1] == '(' {
			return "", false
		}
		if s[i+1] == '{' {
			end := matchBrace(s, i+1)
			if end < 0 {
				return "", false
			}
			v, ok := param(s[i+2:end], lookup)
			if !ok {
				return "", false
			}
			b.WriteString(v)
			i = end
			continue
		}
		j := i + 1
		for j < len(s) && (s[j] == '_' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= '0' && s[j] <= '9') {
			j++
		}
		if j == i+1 {
			b.WriteByte(c)
			continue
		}
		v, _ := lookup(s[i+1 : j])
		b.WriteString(v)
		i = j - 1
	}
	return b.String(), true
}

func matchBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func param(expr string, lookup func(string) (string, bool)) (string, bool) {
	n := 0
	for n < len(expr) && (expr[n] == '_' || expr[n] >= 'A' && expr[n] <= 'Z' || expr[n] >= 'a' && expr[n] <= 'z' || expr[n] >= '0' && expr[n] <= '9') {
		n++
	}
	if n == 0 {
		return "", false
	}
	name, op := expr[:n], expr[n:]
	val, set := lookup(name)
	switch {
	case op == "":
		return val, true
	case strings.HasPrefix(op, ":-"):
		if val != "" {
			return val, true
		}
		return expand(op[2:], lookup)
	case strings.HasPrefix(op, "+"):
		if set {
			return expand(op[1:], lookup)
		}
		return "", true
	case strings.HasPrefix(op, "-"):
		if set {
			return val, true
		}
		return expand(op[1:], lookup)
	case strings.HasPrefix(op, "#"):
		return strings.TrimPrefix(val, op[1:]), true
	}
	return "", false
}
