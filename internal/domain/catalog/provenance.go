package catalog

import (
	"path"
	"regexp"
	"strings"
)

// Locations are the machine paths provenance globs refer to.
type Locations struct {
	// BrewPrefix replaces {brew_prefix}; "" disables globs that use it.
	BrewPrefix string
	// NPMPrefix replaces {npm_prefix}; "" disables globs that use it.
	NPMPrefix string
	// Home replaces a leading ~; "" disables globs that use it.
	Home string
}

// Resolve returns the locations with each non-empty path passed through
// eval (filepath.EvalSymlinks in adapters), keeping a path as is when eval
// fails. Paths resolved through symlinks must be compared with resolved
// locations: on macOS /var is a symlink to /private/var, and a Homebrew
// prefix or home can be a symlink too.
func (l Locations) Resolve(eval func(string) (string, error)) Locations {
	r := func(p string) string {
		if p == "" {
			return ""
		}
		if v, err := eval(p); err == nil {
			return v
		}
		return p
	}
	return Locations{BrewPrefix: r(l.BrewPrefix), NPMPrefix: r(l.NPMPrefix), Home: r(l.Home)}
}

// ExpandGlob fills in {brew_prefix}, {npm_prefix} and a leading ~. It
// reports false when the glob needs a location that is unknown.
func (l Locations) ExpandGlob(glob string) (string, bool) {
	if strings.Contains(glob, "{brew_prefix}") {
		if l.BrewPrefix == "" {
			return "", false
		}
		glob = strings.ReplaceAll(glob, "{brew_prefix}", l.BrewPrefix)
	}
	if strings.Contains(glob, "{npm_prefix}") {
		if l.NPMPrefix == "" {
			return "", false
		}
		glob = strings.ReplaceAll(glob, "{npm_prefix}", l.NPMPrefix)
	}
	if glob == "~" || strings.HasPrefix(glob, "~/") {
		if l.Home == "" {
			return "", false
		}
		glob = l.Home + glob[1:]
	}
	return glob, true
}

// MatchGlob matches a slash-separated path against a glob where ** matches
// any characters including slashes, * and ? match within one segment, and
// a trailing /** also matches the directory itself.
func MatchGlob(glob, p string) bool {
	if strings.HasSuffix(glob, "/**") && path.Clean(p) == path.Clean(strings.TrimSuffix(glob, "/**")) {
		return true
	}
	var re strings.Builder
	re.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; {
		case c == '*' && i+1 < len(glob) && glob[i+1] == '*':
			re.WriteString(".*")
			i++
		case c == '*':
			re.WriteString("[^/]*")
		case c == '?':
			re.WriteString("[^/]")
		default:
			re.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	re.WriteString("$")
	ok, err := regexp.MatchString(re.String(), p)
	return err == nil && ok
}

// provenanceOrder is the order catalog globs are tried in, so the result
// does not depend on map iteration.
var provenanceOrder = []Provenance{ProvBrewCask, ProvBrew, ProvNative, ProvNPM, ProvNVM, ProvFNM, ProvASDF, ProvVolta, ProvScript, ProvAppBundle, ProvOS, ProvUnknown}

// osDirs hold OS-bundled tools (research.md RQ-8).
var osDirs = []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}

// ProvenanceOf says where an install at p (a resolved path) came from:
// first the entry's own provenance globs, then the generic rules (Homebrew
// Caskroom and Cellar/opt, npm global modules, Node version managers, OS
// directories). It returns ProvUnknown when nothing matches.
func ProvenanceOf(e Entry, p string, l Locations) Provenance {
	for _, prov := range provenanceOrder {
		glob, ok := e.Detect.Provenance[prov]
		if !ok {
			continue
		}
		if g, ok := l.ExpandGlob(glob); ok && MatchGlob(g, p) {
			return prov
		}
	}
	return GenericProvenance(p, l)
}

// GenericProvenance applies the location rules that do not depend on the
// catalog entry.
func GenericProvenance(p string, l Locations) Provenance {
	under := func(dir string) bool { return dir != "" && strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/") }
	switch {
	case l.BrewPrefix != "" && under(path.Join(l.BrewPrefix, "Caskroom")):
		return ProvBrewCask
	case l.BrewPrefix != "" && under(path.Join(l.BrewPrefix, "lib", "node_modules")):
		return ProvNPM
	case l.NPMPrefix != "" && under(path.Join(l.NPMPrefix, "lib", "node_modules")):
		return ProvNPM
	case l.BrewPrefix != "" && (under(path.Join(l.BrewPrefix, "Cellar")) || under(path.Join(l.BrewPrefix, "opt"))):
		return ProvBrew
	case l.Home != "" && under(path.Join(l.Home, ".nvm")):
		return ProvNVM
	case l.Home != "" && (under(path.Join(l.Home, ".local", "state", "fnm_multishells")) || under(path.Join(l.Home, ".local", "share", "fnm")) || under(path.Join(l.Home, ".fnm"))):
		return ProvFNM
	case l.Home != "" && under(path.Join(l.Home, ".asdf")):
		return ProvASDF
	case l.Home != "" && under(path.Join(l.Home, ".volta")):
		return ProvVolta
	}
	for _, d := range osDirs {
		if path.Dir(p) == d {
			return ProvOS
		}
	}
	return ProvUnknown
}
