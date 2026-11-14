package brew

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// infoShapeProblems checks the JSON types of every field facman reads.
func infoShapeProblems(data []byte) []string {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return []string{"not a JSON object: " + err.Error()}
	}
	var problems []string
	want := func(where string, obj map[string]any, key, kind string, nullable bool) {
		v, ok := obj[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: missing %q", where, key))
			return
		}
		if v == nil && nullable {
			return
		}
		got := fmt.Sprintf("%T", v)
		ok = map[string]string{"string": "string", "bool": "bool", "array": "[]interface {}"}[kind] == got
		if !ok {
			problems = append(problems, fmt.Sprintf("%s.%s: %s, want %s", where, key, got, kind))
		}
	}
	formulae, _ := doc["formulae"].([]any)
	casks, _ := doc["casks"].([]any)
	if doc["formulae"] == nil || doc["casks"] == nil {
		problems = append(problems, "missing formulae or casks array")
	}
	for i, x := range formulae {
		f, _ := x.(map[string]any)
		where := fmt.Sprintf("formulae[%d]", i)
		for _, k := range []string{"name", "full_name", "tap"} {
			want(where, f, k, "string", k == "tap")
		}
		want(where, f, "keg_only", "bool", false)
		want(where, f, "linked_keg", "string", true)
		want(where, f, "installed", "array", false)
		inst, _ := f["installed"].([]any)
		for j, y := range inst {
			iv, _ := y.(map[string]any)
			want(fmt.Sprintf("%s.installed[%d]", where, j), iv, "version", "string", false)
		}
	}
	for i, x := range casks {
		c, _ := x.(map[string]any)
		where := fmt.Sprintf("casks[%d]", i)
		for _, k := range []string{"token", "full_token", "tap"} {
			want(where, c, k, "string", k == "tap")
		}
		want(where, c, "installed", "string", true)
		want(where, c, "artifacts", "array", false)
		arts, _ := c["artifacts"].([]any)
		for j, y := range arts {
			if a, ok := y.(map[string]any); !ok || len(a) != 1 {
				problems = append(problems, fmt.Sprintf("%s.artifacts[%d]: want a one-key object, got %v", where, j, y))
			}
		}
	}
	return problems
}

// The hand-written fixtures must have the shape the real-Homebrew contract
// test checks (review FR-003), and the checker must catch a changed type.
func TestInfoShape(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "info-*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("fixtures: %v %v", files, err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // test fixture
		if err != nil {
			t.Fatal(err)
		}
		if p := infoShapeProblems(b); len(p) > 0 {
			t.Errorf("%s: %v", f, p)
		}
	}
	bad := []struct{ doc, want string }{
		{`[]`, "not a JSON object"},
		{`{"formulae": []}`, "missing formulae or casks"},
		{`{"formulae": [{"name": "jq", "full_name": "jq", "tap": "homebrew/core", "keg_only": "no", "linked_keg": null, "installed": []}], "casks": []}`, "keg_only: string, want bool"},
		{`{"formulae": [{"name": "jq", "full_name": "jq", "keg_only": false, "linked_keg": null, "installed": [{"version": 1}]}], "casks": []}`, "installed[0].version: float64, want string"},
		{`{"formulae": [], "casks": [{"token": "x", "full_token": "x", "tap": null, "installed": 2, "artifacts": [{"app": [], "pkg": []}]}]}`, "installed: float64"},
		{`{"formulae": [], "casks": [{"token": "x", "full_token": "x", "tap": null, "installed": null, "artifacts": [{"app": [], "pkg": []}]}]}`, "one-key object"},
		{`{"formulae": [], "casks": [{"token": "x", "tap": null, "installed": null, "artifacts": []}]}`, `missing "full_token"`},
	}
	for _, tt := range bad {
		if p := strings.Join(infoShapeProblems([]byte(tt.doc)), "; "); !strings.Contains(p, tt.want) {
			t.Errorf("infoShapeProblems(%s) = %q, want %q", tt.doc, p, tt.want)
		}
	}
}
