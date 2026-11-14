package fakeworld

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newWorld(t *testing.T) *World {
	t.Helper()
	return &World{
		Rules: []Rule{{Cmd: "npm", ArgsPrefix: []string{"ls", "-g"}, Stdout: "{}"}},
		Brew: &Brew{
			Prefix:   t.TempDir(),
			Formulae: map[string]Formula{"sqlite": {Version: "3.47", KegOnly: true}, "gh": {Version: "2.60", Symlinks: map[string]string{"bin/gh": "/abs/gh"}}},
			Casks:    map[string]Cask{"iterm2": {Version: "3.5", Files: []string{"Caskroom/iterm2/3.5/iTerm.app/x"}}},
		},
	}
}

func TestBuiltin(t *testing.T) {
	w := newWorld(t)
	tests := []struct {
		args     []string
		wantExit int
		want     string
		mutated  bool
	}{
		{[]string{"info", "--installed"}, 1, "only brew info --json=v2", false},
		{[]string{"info", "--json=v2"}, 1, "requires a formula argument", false},
		{[]string{"info", "--json=v2", "nope"}, 1, "No available formula", false},
		{[]string{"info", "--json=v2", "--cask", "sqlite"}, 1, "No available formula", false},
		{[]string{"info", "--json=v2", "sqlite"}, 0, `"keg_only":true`, false},
		{[]string{"install"}, 1, "requires at least 1", false},
		{[]string{"install", "nope"}, 1, "no available formula", false},
		{[]string{"install", "--cask", "nope"}, 1, "no available cask", false},
		{[]string{"install", "sqlite"}, 0, "Installed sqlite", true},
		{[]string{"install", "gh"}, 0, "Installed gh", true},
		{[]string{"install", "--cask", "iterm2"}, 0, "Installed iterm2", true},
		{[]string{"info", "--json=v2", "--installed"}, 0, `"linked_keg":null`, false},
		{[]string{"tap", "a/b", "a/b"}, 0, "Tapped a/b", true},
		{[]string{"tap", "a/b"}, 0, "Tapped", false},
		{[]string{"trust", "--cask", "a/b/c"}, 0, "Trusted", true},
		{[]string{"trust", "--cask", "a/b/c"}, 0, "Trusted", false},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			r := w.Builtin("brew", tt.args)
			if r.Exit != tt.wantExit || !strings.Contains(r.Stdout+r.Stderr, tt.want) || r.Mutated != tt.mutated {
				t.Errorf("got %+v, want exit %d %q mutated %v", r, tt.wantExit, tt.want, tt.mutated)
			}
		})
	}
	if r := (&World{}).Builtin("brew", []string{"--prefix"}); r.Exit != 127 {
		t.Errorf("brew without emulation = %+v", r)
	}
	if _, ok := w.Match("npm", []string{"ls"}); ok {
		t.Error("short args matched a longer prefix")
	}
	if _, ok := w.Match("npm", []string{"ls", "-g", "--json"}); !ok {
		t.Error("prefix rule did not match")
	}
}

func TestMaterializeErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	b := &Brew{Prefix: file} // a file, so nothing can be created under it
	if err := b.materialize([]string{"bin/x"}, nil, "1"); err == nil {
		t.Error("want error creating under a file")
	}
	if err := b.materialize(nil, map[string]string{"bin/x": "y"}, "1"); err == nil {
		t.Error("want error linking under a file")
	}
	dir := t.TempDir()
	b = &Brew{Prefix: dir, Formulae: map[string]Formula{"x": {Files: []string{"a"}}}, Casks: map[string]Cask{"y": {Symlinks: map[string]string{"a": "b"}}}}
	if r := b.run([]string{"install", "x"}); r.Exit != 0 {
		t.Fatalf("first install: %+v", r)
	}
	if err := os.Chmod(filepath.Join(dir, "a"), 0o000); err != nil {
		t.Fatal(err)
	}
	if r := b.run([]string{"install", "--cask", "y"}); r.Exit != 1 {
		t.Errorf("symlink over an existing file should fail: %+v", r)
	}
	if err := os.MkdirAll(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := b.materialize([]string{"d"}, nil, "1"); err == nil {
		t.Error("writing over a directory should fail")
	}
}

func TestLoadSaveLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, WorldFile)
	if _, err := Load(path); err == nil {
		t.Error("loading a missing world should fail")
	}
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("loading bad JSON should fail")
	}
	if err := Save(filepath.Join(dir, "missing", "w.json"), World{}); err == nil {
		t.Error("saving into a missing dir should fail")
	}
	if err := Save(path, World{Rules: []Rule{{Cmd: "x"}}}); err != nil {
		t.Fatal(err)
	}
	w, err := Load(path)
	if err != nil || len(w.Rules) != 1 {
		t.Fatalf("round trip: %+v, %v", w, err)
	}
	logPath := filepath.Join(dir, LogFile)
	if log, err := ReadLog(logPath); err != nil || log != nil {
		t.Errorf("missing log = %v, %v", log, err)
	}
	if err := AppendLog(logPath, Invocation{Argv: []string{"brew"}}); err != nil {
		t.Fatal(err)
	}
	if err := AppendLog(filepath.Join(dir, "missing", "l"), Invocation{}); err == nil {
		t.Error("appending into a missing dir should fail")
	}
	if log, err := ReadLog(logPath); err != nil || len(log) != 1 {
		t.Errorf("log = %v, %v", log, err)
	}
	if err := os.WriteFile(logPath, []byte("{bad\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLog(logPath); err == nil {
		t.Error("bad log line should fail")
	}
	if _, err := ReadLog(dir); err == nil {
		t.Error("reading a directory should fail")
	}
	if err := Save(dir, World{}); err == nil {
		t.Error("renaming over a directory should fail")
	}
}
