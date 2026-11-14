package brew

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"
)

// Info is the document `brew info --json=v2` prints (research.md RQ-1).
type Info struct {
	// Formulae are the formulae reported.
	Formulae []Formula `json:"formulae"`
	// Casks are the casks reported.
	Casks []Cask `json:"casks"`
}

// Formula is one formula in a brew info document.
type Formula struct {
	// Name is the short name (jq, node@24).
	Name string `json:"name"`
	// FullName includes the tap for third-party formulae.
	FullName string `json:"full_name"`
	// Tap is the tap the formula comes from.
	Tap string `json:"tap"`
	// Installed lists the installed versions, oldest first.
	Installed []FormulaInstall `json:"installed"`
	// LinkedKeg is the linked version, nil when not linked.
	LinkedKeg *string `json:"linked_keg"`
	// KegOnly formulae are never linked into {prefix}/bin.
	KegOnly bool `json:"keg_only"`
}

// FormulaInstall is one installed version of a formula.
type FormulaInstall struct {
	// Version is the installed version.
	Version string `json:"version"`
}

// Version returns the newest installed version ("" when not installed).
func (f Formula) Version() string {
	if len(f.Installed) == 0 {
		return ""
	}
	return f.Installed[len(f.Installed)-1].Version
}

// Cask is one cask in a brew info document.
type Cask struct {
	// Token is the short token (claude-code, orca).
	Token string `json:"token"`
	// FullToken includes the tap for third-party casks.
	FullToken string `json:"full_token"`
	// Tap is the tap the cask comes from.
	Tap string `json:"tap"`
	// Installed is the installed version, nil when not installed.
	Installed *string `json:"installed"`
	// Artifacts are the cask's stanzas, each a one-key object.
	Artifacts []map[string]json.RawMessage `json:"artifacts"`
}

// Version returns the installed version ("" when not installed).
func (c Cask) Version() string {
	if c.Installed == nil {
		return ""
	}
	return *c.Installed
}

// Apps returns the app bundle names the cask installs.
func (c Cask) Apps() []string { return c.names("app") }

// Binaries returns the names the cask links into {prefix}/bin: binary
// stanzas and command_wrapper scripts.
func (c Cask) Binaries() []string { return c.names("binary", "command_wrapper") }

func (c Cask) names(kinds ...string) []string {
	var out []string
	for _, l := range c.Links() {
		if slices.Contains(kinds, l.Kind) {
			out = append(out, l.Name)
		}
	}
	return out
}

// NeedsSudo reports whether the cask runs a pkg or installer artifact,
// which may prompt for sudo.
func (c Cask) NeedsSudo() bool {
	for _, a := range c.Artifacts {
		if _, ok := a["pkg"]; ok {
			return true
		}
		if _, ok := a["installer"]; ok {
			return true
		}
	}
	return false
}

// Link is one path a cask artifact writes.
type Link struct {
	// Kind is the stanza: app, binary, command_wrapper, bash_completion, …
	Kind string
	// Name is the base name of the link (of Target when there is one).
	Name string
	// Target is the absolute path Homebrew reports for the link. Homebrew 7
	// puts it in a "target" key next to the stanza for an installed cask;
	// it is "" when not reported (a cask that is not installed).
	Target string
}

// linkKinds are the stanzas whose link path facman derives when Homebrew
// reports no target: app bundles go to the app dir, the others to
// {prefix}/bin.
var linkKinds = []string{"app", "binary", "command_wrapper"}

// Links returns the paths the cask's artifacts write, in order. It reads
// both shapes: Homebrew 7's installed form {"<kind>": [source, …],
// "target": "/abs/path"} (any stanza with a target counts), and the
// uninstalled form {"<kind>": [source, {"target": name}]} for app, binary
// and command_wrapper stanzas, whose name is derived (command_wrapper's
// first element is the wrapper's name). An object with more than one
// stanza key, or a target that is not an absolute path without control
// characters, is not trusted as a link path.
func (c Cask) Links() []Link {
	var out []Link
	for _, a := range c.Artifacts {
		kind, raw, ok := stanza(a)
		if !ok {
			continue
		}
		l := Link{Kind: kind, Name: derivedName(kind, raw)}
		if t, ok := siblingTarget(a); ok {
			l.Target, l.Name = t, path.Base(t)
		}
		if l.Target == "" && (l.Name == "" || !slices.Contains(linkKinds, kind)) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// stanza returns the artifact's one stanza key other than "target".
func stanza(a map[string]json.RawMessage) (string, json.RawMessage, bool) {
	kind, n := "", 0
	for k := range a {
		if k != "target" {
			kind, n = k, n+1
		}
	}
	if n != 1 {
		return "", nil, false
	}
	return kind, a[kind], true
}

func siblingTarget(a map[string]json.RawMessage) (string, bool) {
	raw, ok := a["target"]
	if !ok {
		return "", false
	}
	var t string
	if err := json.Unmarshal(raw, &t); err != nil || !path.IsAbs(t) || strings.IndexFunc(t, unicode.IsControl) >= 0 {
		return "", false
	}
	return path.Clean(t), true
}

// derivedName reads [source] or [source, {"target": name}].
func derivedName(kind string, raw json.RawMessage) string {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil || len(parts) == 0 {
		return ""
	}
	var source string
	if err := json.Unmarshal(parts[0], &source); err != nil || source == "" {
		return ""
	}
	name := path.Base(source)
	if kind != "command_wrapper" && len(parts) > 1 {
		var opts struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(parts[1], &opts); err == nil && opts.Target != "" {
			name = path.Base(opts.Target)
		}
	}
	return name
}

// ParseInfo decodes a brew info --json=v2 document.
func ParseInfo(data []byte) (Info, error) {
	var info Info
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&info); err != nil {
		return Info{}, fmt.Errorf("brew info returned invalid JSON: %w", err)
	}
	return info, nil
}

// official reports whether tap is one of Homebrew's own taps, whose items
// are addressed by their short name.
func official(tap string) bool {
	return tap == "" || strings.HasPrefix(tap, "homebrew/")
}

// Formula returns the formula whose full name is id, or, for an id
// without a tap, the official formula with that short name.
func (i Info) Formula(id string) (Formula, bool) {
	for _, f := range i.Formulae {
		if f.FullName == id || (!strings.Contains(id, "/") && f.Name == id && official(f.Tap)) {
			return f, true
		}
	}
	return Formula{}, false
}

// Cask returns the cask whose full token is id, or, for an id without a
// tap, the official cask with that token (so "orca" never matches
// stablyai/orca/orca).
func (i Info) Cask(id string) (Cask, bool) {
	for _, c := range i.Casks {
		if c.FullToken == id || (!strings.Contains(id, "/") && c.Token == id && official(c.Tap)) {
			return c, true
		}
	}
	return Cask{}, false
}

// ParseTaps reads `brew tap` (no arguments): one tap per line.
func ParseTaps(out string) []string {
	var taps []string
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) > 0 && strings.Count(f[0], "/") == 1 {
			taps = append(taps, f[0])
		}
	}
	return taps
}

// ParseTrusted reads `brew trust` (no arguments) and returns every full
// item path (user/repo/name) it lists. The exact format is unconfirmed
// (R-OQ-1): any whitespace-separated token with exactly two slashes counts,
// after trimming punctuation, so unreadable output means "not trusted".
func ParseTrusted(out string) []string {
	var items []string
	for _, tok := range strings.Fields(out) {
		tok = strings.Trim(tok, ",;:()[]\"'")
		if strings.Count(tok, "/") == 2 && !strings.HasPrefix(tok, "/") {
			items = append(items, tok)
		}
	}
	return items
}
