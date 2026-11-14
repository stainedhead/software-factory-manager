package catalog

import "testing"

func TestExpandGlob(t *testing.T) {
	l := Locations{BrewPrefix: "/opt/homebrew", NPMPrefix: "/usr/local", Home: "/Users/j"}
	tests := []struct {
		loc  Locations
		glob string
		want string
		ok   bool
	}{
		{l, "{brew_prefix}/Cellar/jq/**", "/opt/homebrew/Cellar/jq/**", true},
		{l, "{npm_prefix}/lib/node_modules/x/**", "/usr/local/lib/node_modules/x/**", true},
		{l, "~/.local/share/claude/**", "/Users/j/.local/share/claude/**", true},
		{l, "~", "/Users/j", true},
		{l, "/abs/~x", "/abs/~x", true},
		{Locations{}, "{brew_prefix}/x", "", false},
		{Locations{}, "{npm_prefix}/x", "", false},
		{Locations{}, "~/x", "", false},
	}
	for _, tt := range tests {
		got, ok := tt.loc.ExpandGlob(tt.glob)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ExpandGlob(%q) = %q, %v", tt.glob, got, ok)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		glob, path string
		want       bool
	}{
		{"/opt/homebrew/Caskroom/claude-code*/**", "/opt/homebrew/Caskroom/claude-code/2.1.4/claude", true},
		{"/opt/homebrew/Caskroom/claude-code*/**", "/opt/homebrew/Caskroom/claude-code@latest/2/claude", true},
		{"/opt/homebrew/Caskroom/claude-code*/**", "/opt/homebrew/Caskroom/codex/1/codex", false},
		{"/opt/homebrew/opt/node@24/**", "/opt/homebrew/opt/node@24", true},
		{"/Users/j/.local/bin/uv", "/Users/j/.local/bin/uv", true},
		{"/Users/j/.local/bin/uv", "/Users/j/.local/bin/uvx", false},
		{"/a/*/c", "/a/b/c", true},
		{"/a/*/c", "/a/b/x/c", false},
		{"/a/?", "/a/b", true},
		{"/a.b/(x)", "/a.b/(x)", true},
		{"/a.b", "/aXb", false},
	}
	for _, tt := range tests {
		if got := MatchGlob(tt.glob, tt.path); got != tt.want {
			t.Errorf("MatchGlob(%q, %q) = %v", tt.glob, tt.path, got)
		}
	}
}

func TestProvenanceOf(t *testing.T) {
	l := Locations{BrewPrefix: "/opt/homebrew", NPMPrefix: "/Users/j/.npm-global", Home: "/Users/j"}
	claude := Entry{ID: "claude-code", Detect: DetectSpec{Provenance: map[Provenance]string{
		ProvNative:   "~/.local/share/claude/**",
		ProvBrewCask: "{brew_prefix}/Caskroom/claude-code*/**",
		ProvNPM:      "{npm_prefix}/lib/node_modules/@anthropic-ai/claude-code/**",
	}}}
	uv := Entry{ID: "uv", Detect: DetectSpec{Provenance: map[Provenance]string{ProvScript: "~/.local/bin/uv"}}}
	tests := []struct {
		e    Entry
		path string
		want Provenance
	}{
		{claude, "/Users/j/.local/share/claude/versions/2.1.4", ProvNative},
		{claude, "/opt/homebrew/Caskroom/claude-code/2.1.4/claude", ProvBrewCask},
		{claude, "/Users/j/.npm-global/lib/node_modules/@anthropic-ai/claude-code/cli.js", ProvNPM},
		{uv, "/Users/j/.local/bin/uv", ProvScript},
		{Entry{}, "/opt/homebrew/Caskroom/x/1/x", ProvBrewCask},
		{Entry{}, "/opt/homebrew/Cellar/jq/1.7.1/bin/jq", ProvBrew},
		{Entry{}, "/opt/homebrew/opt/node@24/bin/node", ProvBrew},
		{Entry{}, "/opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js", ProvNPM},
		{Entry{}, "/Users/j/.npm-global/lib/node_modules/x/y", ProvNPM},
		{Entry{}, "/Users/j/.nvm/versions/node/v22.1.0/bin/node", ProvNVM},
		{Entry{}, "/Users/j/.local/state/fnm_multishells/123/bin/node", ProvFNM},
		{Entry{}, "/Users/j/.asdf/installs/nodejs/24/bin/node", ProvASDF},
		{Entry{}, "/Users/j/.volta/tools/image/node/24/bin/node", ProvVolta},
		{Entry{}, "/usr/bin/jq", ProvOS},
		{Entry{}, "/bin/git", ProvOS},
		{Entry{}, "/usr/local/bin/aws", ProvUnknown},
		{Entry{}, "/usr/bin/x/y", ProvUnknown},
	}
	for _, tt := range tests {
		if got := ProvenanceOf(tt.e, tt.path, l); got != tt.want {
			t.Errorf("ProvenanceOf(%s, %q) = %s, want %s", tt.e.ID, tt.path, got, tt.want)
		}
	}
	if got := ProvenanceOf(claude, "/x/Caskroom/claude-code/1/claude", Locations{}); got != ProvUnknown {
		t.Errorf("no locations = %s", got)
	}
}
