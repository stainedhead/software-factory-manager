// Command archcheck enforces the Clean Architecture import rules of
// AGENTS.md (review FR-010c). It reads `go list` output on stdin, one
// package per line: the import path followed by its imports (production
// and test), separated by spaces, and exits 1 naming each broken rule.
//
//	go list -f '{{.ImportPath}}{{range .Imports}} {{.}}{{end}}{{range .TestImports}} {{.}}{{end}}' ./... | go run ./scripts/archcheck
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// module is this repository's module path.
const module = "github.com/stainedhead/software-factory-manager"

// Violation is one import that breaks a rule.
type Violation struct {
	Rule, Pkg, Import string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s: %s imports %s", v.Rule, v.Pkg, v.Import)
}

func main() {
	vs, err := checkReader(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "archcheck:", err)
		os.Exit(2)
	}
	os.Exit(report(os.Stdout, vs))
}

// report prints the violations and returns the exit code.
func report(w io.Writer, vs []Violation) int {
	for _, v := range vs {
		_, _ = fmt.Fprintln(w, v)
	}
	if len(vs) > 0 {
		_, _ = fmt.Fprintf(w, "archcheck: %d import(s) break the rules in AGENTS.md (Architecture)\n", len(vs))
		return 1
	}
	return 0
}

// checkReader parses `go list` lines and checks them.
func checkReader(r io.Reader) ([]Violation, error) {
	pkgs := map[string][]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		pkgs[fields[0]] = append(pkgs[fields[0]], fields[1:]...)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading go list output: %w", err)
	}
	return check(pkgs), nil
}

// layer returns the package's layer below the module ("domain",
// "usecase", "adapter/tui", "cmd", "testkit"...), or "" for a package
// outside the module.
func layer(pkg string) string {
	rest, ok := strings.CutPrefix(pkg, module+"/")
	if !ok {
		return ""
	}
	rest = strings.TrimPrefix(rest, "internal/")
	return rest
}

func within(pkg, prefix string) bool {
	l := layer(pkg)
	return l == prefix || strings.HasPrefix(l, prefix+"/")
}

// thirdParty reports whether an import is outside the standard library
// and outside this module.
func thirdParty(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return strings.Contains(first, ".") && !strings.HasPrefix(imp, module)
}

// check applies every rule, sorted for stable output.
func check(pkgs map[string][]string) []Violation {
	var vs []Violation
	for pkg, imps := range pkgs {
		for _, imp := range imps {
			switch {
			case within(pkg, "domain") && layer(imp) != "" && !within(imp, "domain"):
				vs = append(vs, Violation{"domain-imports-outer-layer", pkg, imp})
			case within(pkg, "domain") && thirdParty(imp):
				vs = append(vs, Violation{"domain-imports-third-party", pkg, imp})
			case within(pkg, "usecase") && (within(imp, "adapter") || within(imp, "cmd") || within(imp, "testkit")):
				vs = append(vs, Violation{"usecase-imports-outer-layer", pkg, imp})
			case within(pkg, "adapter/tui") && within(imp, "adapter/cli"), within(pkg, "adapter/cli") && within(imp, "adapter/tui"):
				vs = append(vs, Violation{"front-ends-import-each-other", pkg, imp})
			}
		}
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i].String() < vs[j].String() })
	return compact(vs)
}

// compact drops duplicates (an import listed by Imports and TestImports).
func compact(vs []Violation) []Violation {
	out := vs[:0]
	for i, v := range vs {
		if i == 0 || v != vs[i-1] {
			out = append(out, v)
		}
	}
	return out
}
