package main

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"
)

const goodNote = "# Guide\n\n> **Status: planned for Increment 3.** Not yet: `-cl` exits 64 until then.\n\nBody\n"

func repo(makefile, agents string, guide string) fstest.MapFS {
	fsys := fstest.MapFS{
		"Makefile":  {Data: []byte(makefile)},
		"AGENTS.md": {Data: []byte(agents)},
	}
	for _, g := range FutureGuides {
		fsys[g] = &fstest.MapFile{Data: []byte(goodNote)}
	}
	if guide != "" {
		fsys[FutureGuides[0]] = &fstest.MapFile{Data: []byte(guide)}
	}
	return fsys
}

const agentsOK = "# A\n\n## Commands\n\n| Command | Purpose |\n|---|---|\n| `make build` | b |\n| `make fuzz` (`FUZZTIME=30s`) | f |\n\n## Next\n| `make lint` | outside the table |\n"

func TestCheck(t *testing.T) {
	tests := []struct {
		name, makefile, agents, guide, want string
	}{
		{"all listed", ".PHONY: build fuzz\n", agentsOK, "", ""},
		{"target missing", ".PHONY: build fuzz lint\n", agentsOK, "", "no row for make lint"},
		{"no commands section", ".PHONY: build\n", "# A\n", "", "no row for make build"},
		{"no phony", "build:\n", agentsOK, "", "no .PHONY line"},
		{"guide without note", ".PHONY: build\n", agentsOK, "# Guide\n\nBody\n", "configuration-list.md: no status note"},
		{"note too late", ".PHONY: build\n", agentsOK, "# Guide\n\nIntro\n" + goodNote, "configuration-list.md: no status note"},
		{"note without exit code", ".PHONY: build\n", agentsOK, "# G\n\n> **Status: planned for Increment 3.** Soon.\n", "no status note"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems, err := check(repo(tt.makefile, tt.agents, tt.guide))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := report(&out, problems)
			if tt.want == "" {
				if code != 0 {
					t.Errorf("exit %d: %s", code, out.String())
				}
				return
			}
			if code != 1 || !strings.Contains(out.String(), tt.want) {
				t.Errorf("exit %d, output %q; want 1 and %q", code, out.String(), tt.want)
			}
		})
	}
}

func TestCheckMissingFiles(t *testing.T) {
	for _, drop := range []string{"Makefile", "AGENTS.md", FutureGuides[2]} {
		fsys := repo(".PHONY: build\n", agentsOK, "")
		delete(fsys, drop)
		if _, err := check(fsys); err == nil {
			t.Errorf("no error without %s", drop)
		}
	}
}
