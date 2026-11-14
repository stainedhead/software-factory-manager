// Package fakeworld is the state and behaviour of fakecmd, the fake
// multi-call executable used by tests in place of brew, xcode-select, mdfind,
// plutil, npm, getconf, ldd and file. It is test support only: nothing in the
// product imports it.
//
// A World is stored as JSON in a directory next to the fakecmd links. Rules
// answer any command with fixed output; brew calls with no matching rule are
// emulated against World.Brew (install, tap, trust, info --json=v2,
// shellenv, --prefix, --version), so an install changes later answers.
package fakeworld

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// File names inside a fake bin directory.
const (
	WorldFile = "fakecmd-world.json"
	LogFile   = "fakecmd-log.jsonl"
	LockFile  = "fakecmd.lock"
	// DirEnv overrides the directory that holds the world and log.
	DirEnv = "FAKECMD_DIR"
)

// World is everything fakecmd knows.
type World struct {
	// Rules are checked in order before any built-in emulation.
	Rules []Rule `json:"rules,omitempty"`
	// Brew is the emulated Homebrew; nil means brew answers only from rules.
	Brew *Brew `json:"brew,omitempty"`
}

// Rule answers one command. Args nil matches any arguments; ArgsPrefix
// matches arguments that start with it.
type Rule struct {
	// Cmd is the program name (base name of argv[0]).
	Cmd string `json:"cmd"`
	// Args must equal the arguments exactly, when set.
	Args []string `json:"args,omitempty"`
	// ArgsPrefix must prefix the arguments, when set.
	ArgsPrefix []string `json:"args_prefix,omitempty"`
	// Stdout is printed.
	Stdout string `json:"stdout,omitempty"`
	// Stderr is printed.
	Stderr string `json:"stderr,omitempty"`
	// Exit is the exit code.
	Exit int `json:"exit,omitempty"`
	// SleepMS delays the answer.
	SleepMS int `json:"sleep_ms,omitempty"`
	// Builtin runs the built-in brew emulation after SleepMS instead of
	// printing Stdout/Stderr/Exit (for slow installs).
	Builtin bool `json:"builtin,omitempty"`
}

// Matches reports whether the rule answers name with args.
func (r Rule) Matches(name string, args []string) bool {
	if r.Cmd != name {
		return false
	}
	if r.Args != nil && !slices.Equal(r.Args, args) {
		return false
	}
	if r.ArgsPrefix != nil && (len(args) < len(r.ArgsPrefix) || !slices.Equal(r.ArgsPrefix, args[:len(r.ArgsPrefix)])) {
		return false
	}
	return true
}

// Brew is the emulated Homebrew installation.
type Brew struct {
	// Prefix is the Homebrew prefix (a temp dir in tests).
	Prefix string `json:"prefix"`
	// Version is printed by brew --version.
	Version string `json:"version,omitempty"`
	// Formulae are the installable formulae by full name.
	Formulae map[string]Formula `json:"formulae,omitempty"`
	// Casks are the installable casks by full token.
	Casks map[string]Cask `json:"casks,omitempty"`
	// Installed is what is installed now.
	Installed Installed `json:"installed"`
	// Taps are the tapped taps.
	Taps []string `json:"taps,omitempty"`
	// Trusted are the trusted items (or, wrongly, whole taps).
	Trusted []string `json:"trusted,omitempty"`
}

// Formula is an installable formula.
type Formula struct {
	// Version is installed by brew install.
	Version string `json:"version"`
	// KegOnly formulae are not linked.
	KegOnly bool `json:"keg_only,omitempty"`
	// Files are created (relative to the prefix) on install, as executables.
	Files []string `json:"files,omitempty"`
	// Symlinks are created on install: link path -> target, relative to the prefix.
	Symlinks map[string]string `json:"symlinks,omitempty"`
}

// Cask is an installable cask.
type Cask struct {
	// Version is installed by brew install --cask.
	Version string `json:"version"`
	// Artifacts are reported by brew info --json=v2 (app, binary, pkg, installer).
	Artifacts []map[string]any `json:"artifacts,omitempty"`
	// Files are created (relative to the prefix) on install.
	Files []string `json:"files,omitempty"`
	// Symlinks are created on install: link path -> target, relative to the prefix.
	Symlinks map[string]string `json:"symlinks,omitempty"`
}

// Installed maps installed names to versions.
type Installed struct {
	// Formulae maps full names to versions.
	Formulae map[string]string `json:"formulae,omitempty"`
	// Casks maps full tokens to versions.
	Casks map[string]string `json:"casks,omitempty"`
}

// Invocation is one recorded call.
type Invocation struct {
	// Argv starts with the program's base name.
	Argv []string `json:"argv"`
	// EnvNames are the sorted names of every environment variable.
	EnvNames []string `json:"env_names"`
	// Cwd is the working directory.
	Cwd string `json:"cwd"`
}

// Result is fakecmd's answer.
type Result struct {
	// Stdout is printed to standard output.
	Stdout string
	// Stderr is printed to standard error.
	Stderr string
	// Exit is the exit code.
	Exit int
	// Mutated is true when the world changed and must be saved.
	Mutated bool
}

// Load reads a world file.
func Load(path string) (World, error) {
	var w World
	b, err := os.ReadFile(path)
	if err != nil {
		return w, fmt.Errorf("reading world: %w", err)
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return w, fmt.Errorf("decoding world %s: %w", path, err)
	}
	return w, nil
}

// Save writes a world file atomically.
func Save(path string, w World) error {
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding world: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("writing world: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replacing world: %w", err)
	}
	return nil
}

// AppendLog appends one invocation to a JSON-lines log.
func AppendLog(path string, inv Invocation) error {
	b, err := json.Marshal(inv)
	if err != nil {
		return fmt.Errorf("encoding invocation: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening log: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing log: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing log: %w", err)
	}
	return nil
}

// ReadLog reads a JSON-lines log; a missing file is an empty log.
func ReadLog(path string) ([]Invocation, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading log: %w", err)
	}
	var out []Invocation
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var inv Invocation
		if err := json.Unmarshal([]byte(line), &inv); err != nil {
			return nil, fmt.Errorf("decoding log line %q: %w", line, err)
		}
		out = append(out, inv)
	}
	return out, nil
}

// Match returns the first rule that answers name with args.
func (w *World) Match(name string, args []string) (Rule, bool) {
	for _, r := range w.Rules {
		if r.Matches(name, args) {
			return r, true
		}
	}
	return Rule{}, false
}

// Builtin runs the built-in emulation (brew only) and may change w.
func (w *World) Builtin(name string, args []string) Result {
	if name != "brew" || w.Brew == nil {
		return Result{Exit: 127, Stderr: fmt.Sprintf("fakecmd: no rule for %s %s\n", name, strings.Join(args, " "))}
	}
	return w.Brew.run(args)
}

func (b *Brew) run(args []string) Result {
	if len(args) == 0 {
		return Result{Exit: 1, Stderr: "Example usage: brew install FORMULA\n"}
	}
	switch args[0] {
	case "--prefix":
		return Result{Stdout: b.Prefix + "\n"}
	case "--version":
		return Result{Stdout: "Homebrew " + orDefault(b.Version, "6.0.0") + "\n"}
	case "shellenv":
		return Result{Stdout: b.shellenv()}
	case "info":
		return b.info(args[1:])
	case "install":
		return b.install(args[1:])
	case "tap":
		return b.tap(args[1:])
	case "trust":
		return b.trust(args[1:])
	}
	return Result{Exit: 1, Stderr: fmt.Sprintf("fakecmd: unsupported brew command %q\n", args[0])}
}

func (b *Brew) shellenv() string {
	p := b.Prefix
	return fmt.Sprintf("export HOMEBREW_PREFIX=\"%s\";\nexport HOMEBREW_CELLAR=\"%s/Cellar\";\nexport HOMEBREW_REPOSITORY=\"%s\";\nexport PATH=\"%s/bin:%s/sbin${PATH+:$PATH}\";\n", p, p, p, p, p)
}

func splitFlags(args []string) (flags map[string]bool, names []string) {
	flags = map[string]bool{}
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flags[a] = true
			continue
		}
		names = append(names, a)
	}
	return flags, names
}

func tapOf(item string) string {
	parts := strings.Split(item, "/")
	if len(parts) == 3 {
		return parts[0] + "/" + parts[1]
	}
	return ""
}

type infoFormula struct {
	Name      string             `json:"name"`
	FullName  string             `json:"full_name"`
	Tap       string             `json:"tap"`
	Installed []infoFormulaInstl `json:"installed"`
	LinkedKeg *string            `json:"linked_keg"`
	KegOnly   bool               `json:"keg_only"`
}

type infoFormulaInstl struct {
	Version            string `json:"version"`
	InstalledOnRequest bool   `json:"installed_on_request"`
}

type infoCask struct {
	Token     string           `json:"token"`
	FullToken string           `json:"full_token"`
	Tap       string           `json:"tap"`
	Installed *string          `json:"installed"`
	Artifacts []map[string]any `json:"artifacts"`
}

type infoDoc struct {
	Formulae []infoFormula `json:"formulae"`
	Casks    []infoCask    `json:"casks"`
}

func (b *Brew) formulaInfo(name string) infoFormula {
	f := b.Formulae[name]
	out := infoFormula{Name: lastSegment(name), FullName: name, Tap: orDefault(tapOf(name), "homebrew/core"), KegOnly: f.KegOnly, Installed: []infoFormulaInstl{}}
	if v, ok := b.Installed.Formulae[name]; ok {
		out.Installed = append(out.Installed, infoFormulaInstl{Version: v, InstalledOnRequest: true})
		if !f.KegOnly {
			vv := v
			out.LinkedKeg = &vv
		}
	}
	return out
}

func (b *Brew) caskInfo(token string) infoCask {
	c := b.Casks[token]
	out := infoCask{Token: lastSegment(token), FullToken: token, Tap: orDefault(tapOf(token), "homebrew/cask"), Artifacts: c.Artifacts}
	if out.Artifacts == nil {
		out.Artifacts = []map[string]any{}
	}
	if v, ok := b.Installed.Casks[token]; ok {
		vv := v
		out.Installed = &vv
	}
	return out
}

func (b *Brew) info(args []string) Result {
	flags, names := splitFlags(args)
	if !flags["--json=v2"] {
		return Result{Exit: 1, Stderr: "fakecmd: only brew info --json=v2 is emulated\n"}
	}
	doc := infoDoc{Formulae: []infoFormula{}, Casks: []infoCask{}}
	switch {
	case flags["--installed"]:
		for _, n := range sortedKeys(b.Installed.Formulae) {
			doc.Formulae = append(doc.Formulae, b.formulaInfo(n))
		}
		for _, n := range sortedKeys(b.Installed.Casks) {
			doc.Casks = append(doc.Casks, b.caskInfo(n))
		}
	case len(names) == 0:
		return Result{Exit: 1, Stderr: "Error: This command requires a formula argument\n"}
	default:
		for _, n := range names {
			if tap := tapOf(n); tap != "" && !slices.Contains(b.Taps, tap) {
				return Result{Exit: 1, Stderr: fmt.Sprintf("Error: No available formula or cask with the name %q.\n", n)}
			}
			if _, ok := b.Casks[n]; ok && !flags["--formula"] {
				doc.Casks = append(doc.Casks, b.caskInfo(n))
				continue
			}
			if _, ok := b.Formulae[n]; ok && !flags["--cask"] {
				doc.Formulae = append(doc.Formulae, b.formulaInfo(n))
				continue
			}
			return Result{Exit: 1, Stderr: fmt.Sprintf("Error: No available formula or cask with the name %q.\n", n)}
		}
	}
	out, _ := json.Marshal(doc) // cannot fail: plain data
	return Result{Stdout: string(out) + "\n"}
}

func (b *Brew) install(args []string) Result {
	flags, names := splitFlags(args)
	if len(names) == 0 {
		return Result{Exit: 1, Stderr: "Error: Invalid usage: This command requires at least 1 formula or cask argument.\n"}
	}
	res := Result{}
	for _, n := range names {
		if tap := tapOf(n); tap != "" {
			if !slices.Contains(b.Taps, tap) {
				return failInstall(res, fmt.Sprintf("Error: No available formula or cask with the name %q. Tap %s first.\n", n, tap))
			}
			if !slices.Contains(b.Trusted, n) && !slices.Contains(b.Trusted, tap) {
				return failInstall(res, fmt.Sprintf("Error: %s is from an untrusted tap. Run brew trust to trust it.\n", n))
			}
		}
		var err error
		if flags["--cask"] {
			err = b.installCask(n)
		} else {
			err = b.installFormula(n)
		}
		if err != nil {
			return failInstall(res, "Error: "+err.Error()+"\n")
		}
		res.Mutated = true
		res.Stdout += fmt.Sprintf("==> Installed %s\n", n)
	}
	return res
}

func failInstall(res Result, msg string) Result {
	res.Exit, res.Stderr = 1, msg
	return res
}

func (b *Brew) installFormula(n string) error {
	f, ok := b.Formulae[n]
	if !ok {
		return fmt.Errorf("no available formula with the name %q", n)
	}
	if b.Installed.Formulae == nil {
		b.Installed.Formulae = map[string]string{}
	}
	b.Installed.Formulae[n] = f.Version
	return b.materialize(f.Files, f.Symlinks, f.Version)
}

func (b *Brew) installCask(n string) error {
	c, ok := b.Casks[n]
	if !ok {
		return fmt.Errorf("no available cask with the name %q", n)
	}
	if b.Installed.Casks == nil {
		b.Installed.Casks = map[string]string{}
	}
	b.Installed.Casks[n] = c.Version
	return b.materialize(c.Files, c.Symlinks, c.Version)
}

// materialize creates install files under the prefix. Files print the
// version when run with any arguments.
func (b *Brew) materialize(files []string, links map[string]string, version string) error {
	for _, f := range files {
		p := filepath.Join(b.Prefix, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(p), err)
		}
		script := "#!/bin/sh\necho " + version + "\n"
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil { //nolint:gosec // test fixture executable
			return fmt.Errorf("creating %s: %w", p, err)
		}
	}
	for _, link := range sortedKeys(links) {
		p := filepath.Join(b.Prefix, link)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(p), err)
		}
		target := links[link]
		if !filepath.IsAbs(target) {
			target = filepath.Join(b.Prefix, target)
		}
		if err := os.Symlink(target, p); err != nil {
			return fmt.Errorf("linking %s: %w", p, err)
		}
	}
	return nil
}

func (b *Brew) tap(args []string) Result {
	_, names := splitFlags(args)
	if len(names) == 0 {
		var sb strings.Builder
		for _, t := range b.Taps {
			sb.WriteString(t + "\n")
		}
		return Result{Stdout: sb.String()}
	}
	res := Result{}
	for _, n := range names {
		if !slices.Contains(b.Taps, n) {
			b.Taps = append(b.Taps, n)
			res.Mutated = true
		}
		res.Stdout += fmt.Sprintf("==> Tapped %s\n", n)
	}
	return res
}

func (b *Brew) trust(args []string) Result {
	_, names := splitFlags(args)
	if len(names) == 0 {
		var sb strings.Builder
		for _, t := range b.Trusted {
			sb.WriteString(t + "\n")
		}
		return Result{Stdout: sb.String()}
	}
	res := Result{}
	for _, n := range names {
		if !slices.Contains(b.Trusted, n) {
			b.Trusted = append(b.Trusted, n)
			res.Mutated = true
		}
		res.Stdout += fmt.Sprintf("==> Trusted %s\n", n)
	}
	return res
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
