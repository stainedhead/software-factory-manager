package catalogyaml

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

var increment1 = catalog.BackendSet{catalog.BackendBrewFormula: true, catalog.BackendBrewCask: true}

func loadEmbedded(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := Embedded("1.2.3").Load(context.Background())
	if err != nil {
		t.Fatalf("embedded catalog: %v", err)
	}
	return c
}

func entry(t *testing.T, c catalog.Catalog, id string) catalog.Entry {
	t.Helper()
	e, ok := c.Entry(id)
	if !ok {
		t.Fatalf("catalog has no %s", id)
	}
	return e
}

func TestEmbeddedCatalogContents(t *testing.T) {
	c := loadEmbedded(t)
	if c.Revision != "embedded:1.2.3" {
		t.Errorf("revision = %q", c.Revision)
	}
	if got := Embedded("").revision; got != "embedded:dev" {
		t.Errorf("empty version revision = %q", got)
	}
	fr040 := []string{"claude-code", "claude-desktop", "copilot-cli", "github-copilot-app", "antigravity-cli", "codex",
		"hermes-agent", "orca", "t3-code", "paperclip", "git", "gh", "tmux", "visual-studio-code", "iterm2", "sqlite",
		"node", "awscli", "uv", "jq", "ripgrep", "container-runtime", "session-manager-plugin", "gcloud-cli", "azure-cli"}
	extras := []string{"orbstack", "colima", "docker-desktop", "docker", "docker-compose", "gemini-cli"}
	for _, id := range append(fr040, extras...) {
		entry(t, c, id)
	}
	if len(c.Entries) != len(fr040)+len(extras) {
		t.Errorf("catalog has %d entries, want %d", len(c.Entries), len(fr040)+len(extras))
	}
	gemini := entry(t, c, "gemini-cli")
	if gemini.SupersededBy != "antigravity-cli" || len(gemini.Install) != 0 || gemini.Detect.NPMPackage != "@google/gemini-cli" ||
		!slices.Equal(gemini.Detect.Commands, []string{"gemini"}) {
		t.Errorf("gemini-cli = %+v, want detection-only, superseded by antigravity-cli", gemini)
	}
	for _, p := range []platform.Platform{{OS: "darwin"}, {OS: "linux", Display: true}, {OS: "linux", WSL: true}} {
		if a := catalog.SelectInstall(gemini, p, increment1); a.Offered || a.Available {
			t.Errorf("gemini-cli offered on %s: %+v", p.Key(), a)
		}
	}
	if _, ok := c.Entry("hermes"); ok {
		t.Error("bare hermes must not be a catalog entry")
	}

	orca := entry(t, c, "orca").Install[platform.Darwin][0]
	if orca.ID != "stablyai/orca/orca" || orca.Tap != "stablyai/orca" || orca.Trust != catalog.TrustItem || orca.Backend != catalog.BackendBrewCask {
		t.Errorf("orca step = %+v", orca)
	}
	if h := entry(t, c, "hermes-agent").Install[platform.Darwin][0]; h.ID != "hermes-agent" {
		t.Errorf("hermes step = %+v", h)
	}
	if got := entry(t, c, "iterm2").Platforms; !slices.Equal(got, []platform.Key{platform.Darwin}) {
		t.Errorf("iterm2 platforms = %v", got)
	}
	for _, k := range []platform.Key{platform.Darwin, platform.Linux} {
		for _, s := range entry(t, c, "paperclip").Install[k] {
			if s.Backend != catalog.BackendNPM {
				t.Errorf("paperclip %s step %+v is not npm", k, s)
			}
		}
	}
	signIn := []string{}
	for _, e := range c.Entries {
		if e.SignInRequired {
			signIn = append(signIn, e.ID)
		}
	}
	if !slices.Equal(signIn, []string{"claude-code", "claude-desktop", "copilot-cli", "github-copilot-app", "antigravity-cli"}) {
		t.Errorf("sign_in_required = %v", signIn)
	}
	node := entry(t, c, "node")
	if node.Install[platform.Darwin][0].ID != "node@24" || node.SatisfiesIf == nil || node.SatisfiesIf.MinVersion != "24.11" {
		t.Errorf("node = %+v", node)
	}
	if aws := entry(t, c, "awscli"); aws.RejectIf == nil || aws.RejectIf.VersionLT != "2.0" {
		t.Errorf("awscli reject_if = %+v", aws.RejectIf)
	}
	if g := entry(t, c, "container-runtime"); !g.SatisfiedByAnyPresent || !slices.Equal(g.MemberIDs(), []string{"orbstack", "colima", "docker-desktop"}) {
		t.Errorf("container-runtime = %+v", g)
	}
	colima := entry(t, c, "colima")
	if len(colima.Requires) != 2 || colima.Requires[0].Package != "docker" || colima.Requires[1].Package != "docker-compose" {
		t.Errorf("colima requires = %+v", colima.Requires)
	}
	if smp := entry(t, c, "session-manager-plugin"); len(smp.Requires) != 1 || smp.Requires[0].Package != "awscli" {
		t.Errorf("session-manager-plugin requires = %+v", smp.Requires)
	}
	if p := entry(t, c, "paperclip"); p.Service == nil || p.Requires[0].MinVersion != "24.11" {
		t.Errorf("paperclip = %+v", p)
	}
	if bid := entry(t, c, "t3-code").Detect.BundleIDs; len(bid) != 1 {
		t.Errorf("t3-code must be matched by bundle id: %v", bid)
	}
}

func TestEmbeddedCatalogNeverForces(t *testing.T) {
	raw, err := embedded.ReadFile("embedded/catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		code, _, _ := strings.Cut(line, "#")
		if strings.Contains(code, "--force") || strings.Contains(code, "link:") || strings.Contains(code, "--overwrite") {
			t.Errorf("catalog line uses force/link: %q", line)
		}
	}
	c := loadEmbedded(t)
	for _, e := range c.Entries {
		for k, steps := range e.Install {
			for _, s := range steps {
				if s.Trust != "" && strings.Count(s.ID, "/") != 2 {
					t.Errorf("%s %s: trust on %q, which is not a full item path", e.ID, k, s.ID)
				}
			}
		}
	}
}

func TestEmbeddedAvailability(t *testing.T) {
	c := loadEmbedded(t)
	darwin := platform.Platform{OS: "darwin", Display: true}
	linux := platform.Platform{OS: "linux", Display: true}
	headless := platform.Platform{OS: "linux"}
	wsl := platform.Platform{OS: "linux", WSL: true}
	tests := []struct {
		id            string
		p             platform.Platform
		offered, avai bool
		reason        string
	}{
		{"claude-code", darwin, true, true, ""},
		{"claude-code", wsl, true, true, ""},
		{"paperclip", darwin, true, false, "needs the npm backend (Increment 2)"},
		{"iterm2", linux, false, false, ""},
		{"claude-desktop", linux, true, false, "not installable through Homebrew"},
		{"claude-desktop", wsl, true, false, "Windows host"},
		{"github-copilot-app", wsl, true, false, "Windows host"},
		{"github-copilot-app", headless, true, false, "no display"},
		{"visual-studio-code", wsl, true, false, "Windows host"},
		{"session-manager-plugin", linux, true, false, "not installable through Homebrew"},
		{"orca", darwin, true, true, ""},
		{"orbstack", linux, false, false, ""},
		{"docker-desktop", linux, false, false, ""},
		{"docker-desktop", wsl, true, false, "Windows host"},
		{"node", linux, true, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.id+"/"+string(tt.p.Key()), func(t *testing.T) {
			got := catalog.SelectInstall(entry(t, c, tt.id), tt.p, increment1)
			if got.Offered != tt.offered || got.Available != tt.avai || !strings.Contains(got.Reason, tt.reason) {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestEmbeddedProfiles(t *testing.T) {
	c := loadEmbedded(t)
	if got := c.ProfileNames(); !slices.Equal(got, []string{"agentic-factory", "base-dev"}) {
		t.Fatalf("profiles = %v", got)
	}
	d, err := catalog.Resolve(c, []string{"agentic-factory"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseDev := []string{"git", "gh", "tmux", "visual-studio-code", "iterm2", "sqlite", "node", "awscli"}
	for _, id := range baseDev {
		it, ok := d.Item(id)
		if !ok || !it.Required || !slices.Equal(it.FromProfiles, []string{"base-dev"}) {
			t.Errorf("base-dev %s = %+v, %v", id, it, ok)
		}
	}
	onByDefault := []string{"claude-desktop", "claude-code", "copilot-cli", "antigravity-cli", "codex", "uv", "jq", "ripgrep", "orca", "paperclip", "hermes-agent"}
	for _, id := range onByDefault {
		if it, _ := d.Item(id); !it.Required || !it.Selected {
			t.Errorf("%s should be on by default: %+v", id, it)
		}
	}
	optional := []string{"github-copilot-app", "container-runtime", "session-manager-plugin", "gcloud-cli", "azure-cli", "t3-code"}
	for _, id := range optional {
		if it, ok := d.Item(id); !ok || it.Required || it.Selected {
			t.Errorf("%s should be optional and off: %+v", id, it)
		}
	}
	if got := len(d.SelectedIDs()); got != len(baseDev)+len(onByDefault) {
		t.Errorf("selected %d: %v", got, d.SelectedIDs())
	}

	with, err := catalog.Resolve(c, []string{"agentic-factory"}, []string{"gcloud-cli", "colima"})
	if err != nil {
		t.Fatal(err)
	}
	if it, _ := with.Item("gcloud-cli"); !it.Selected {
		t.Error("--with gcloud-cli did not select it")
	}
	for _, id := range []string{"colima", "docker", "docker-compose"} {
		if it, _ := with.Item(id); !it.Selected {
			t.Errorf("--with colima did not select %s", id)
		}
	}
	if _, err := catalog.Resolve(c, []string{"agentic-factory"}, []string{"container-runtime"}); err == nil ||
		!strings.Contains(err.Error(), "orbstack, colima, docker-desktop") {
		t.Errorf("--with container-runtime: %v", err)
	}
}

func writeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const dirCatalog = "apiVersion: facman/v1\npackages:\n  hello:\n    kind: cli\n    detect: { commands: [hello] }\n    install:\n      darwin: [{ backend: brew-formula, id: hello }]\n      linux: [{ backend: brew-formula, id: hello }]\n"
const dirProfile = "apiVersion: facman/v1\nprofile: integration\npackages: [hello]\n"

func TestDirSource(t *testing.T) {
	dir := writeDir(t, map[string]string{"catalog.yaml": dirCatalog, "profiles/integration.yaml": dirProfile, "profiles/README.md": "ignored", "profiles/sub/x.yaml": "ignored"})
	if err := os.MkdirAll(filepath.Join(dir, "profiles", "nested.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := src.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entries) != 1 || c.Entries[0].ID != "hello" {
		t.Errorf("entries = %+v", c.Entries)
	}
	if _, ok := c.Profiles["integration"]; !ok || len(c.Profiles) != 1 {
		t.Errorf("profiles = %v (the embedded catalog must be replaced, not merged)", c.Profiles)
	}
	abs, _ := filepath.Abs(dir)
	if !strings.HasPrefix(c.Revision, "dir:"+abs+"@") || len(c.Revision) != len("dir:"+abs+"@")+64 {
		t.Errorf("revision = %q", c.Revision)
	}
	again, _ := src.Load(context.Background())
	if again.Revision != c.Revision {
		t.Error("revision is not stable across loads")
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles", "integration.yaml"), []byte(dirProfile+"description: changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, _ := src.Load(context.Background())
	if changed.Revision == c.Revision {
		t.Error("revision did not change with the files")
	}
}

func TestDirSourceErrors(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantCode int
		want     []string
	}{
		{"missing catalog.yaml", map[string]string{"profiles/a.yaml": dirProfile}, 3, []string{"missing", "catalog.yaml"}},
		{"missing profiles dir", map[string]string{"catalog.yaml": dirCatalog}, 3, []string{"missing", "profiles"}},
		{"yaml error with position", map[string]string{"catalog.yaml": "apiVersion: facman/v1\npackages:\n  hello:\n    kind: cli\n    detekt: {}\n", "profiles/integration.yaml": dirProfile}, 3, []string{"catalog.yaml:5:5 packages.hello.detekt: unknown field"}},
		{"profile error", map[string]string{"catalog.yaml": dirCatalog, "profiles/integration.yaml": "profile: integration\nbad: 1\n"}, 3, []string{"profiles/integration.yaml:2:1 bad: unknown field"}},
		{"profile api version", map[string]string{"catalog.yaml": dirCatalog, "profiles/integration.yaml": "apiVersion: v0\nprofile: integration\n"}, 3, []string{`profiles/integration.yaml: apiVersion is "v0"`}},
		{"profile name mismatch", map[string]string{"catalog.yaml": dirCatalog, "profiles/other.yaml": dirProfile}, 3, []string{`profile is "integration", but the file is named for "other"`}},
		{"unknown id in profile", map[string]string{"catalog.yaml": dirCatalog, "profiles/integration.yaml": "apiVersion: facman/v1\nprofile: integration\npackages: [nope]\n"}, 3, []string{`unknown package "nope"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := Dir(writeDir(t, tt.files))
			if err != nil {
				t.Fatal(err)
			}
			_, err = src.Load(context.Background())
			if got := failure.ExitCode(err); got != tt.wantCode {
				t.Fatalf("exit = %d, want %d (%v)", got, tt.wantCode, err)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestDirSourceMissingDir(t *testing.T) {
	src, err := Dir(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Load(context.Background())
	if failure.ExitCode(err) != 3 || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v", err)
	}
}

func TestDirSourceUnreadable(t *testing.T) {
	dir := writeDir(t, map[string]string{"catalog.yaml": dirCatalog, "profiles/integration.yaml": dirProfile})
	if err := os.Remove(filepath.Join(dir, "catalog.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "catalog.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, _ := Dir(dir)
	if _, err := src.Load(context.Background()); failure.ExitCode(err) != 3 || !strings.Contains(err.Error(), "cannot read") {
		t.Errorf("err = %v", err)
	}
}

func TestDirRejectsGitURLs(t *testing.T) {
	for _, u := range []string{"https://github.com/org/catalog", "git@github.com:org/catalog.git", "ssh://git@host/x", ""} {
		_, err := Dir(u)
		if failure.ExitCode(err) != 64 {
			t.Errorf("Dir(%q) exit = %d (%v)", u, failure.ExitCode(err), err)
		}
	}
	if _, err := Dir("https://x"); !strings.Contains(err.Error(), "git catalogs arrive in a later increment") {
		t.Errorf("err = %v", err)
	}
}

func TestLoadHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Embedded("x").Load(ctx); err == nil {
		t.Error("want a context error")
	}
}
