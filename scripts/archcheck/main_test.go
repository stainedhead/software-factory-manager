package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCheckRules(t *testing.T) {
	m := module + "/internal/"
	tests := []struct {
		name, line, rule string
	}{
		{"domain imports usecase", m + "domain/plan " + m + "usecase/process", "domain-imports-outer-layer"},
		{"domain imports adapter", m + "domain/plan " + m + "adapter/brew", "domain-imports-outer-layer"},
		{"domain imports testkit", m + "domain/catalog " + m + "testkit", "domain-imports-outer-layer"},
		{"domain imports third party", m + "domain/catalog gopkg.in/yaml.v3", "domain-imports-third-party"},
		{"usecase imports adapter", m + "usecase/scan " + m + "adapter/detect", "usecase-imports-outer-layer"},
		{"usecase imports cmd", m + "usecase/scan " + module + "/cmd/facman", "usecase-imports-outer-layer"},
		{"usecase imports testkit", m + "usecase/execute " + m + "testkit/fakeworld", "usecase-imports-outer-layer"},
		{"tui imports cli", m + "adapter/tui " + m + "adapter/cli", "front-ends-import-each-other"},
		{"cli imports tui", m + "adapter/cli " + m + "adapter/tui", "front-ends-import-each-other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs, err := checkReader(strings.NewReader(tt.line + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if code := report(&out, vs); code != 1 || !strings.Contains(out.String(), tt.rule+":") {
				t.Errorf("exit %d, output %q; want 1 and rule %s", code, out.String(), tt.rule)
			}
		})
	}
}

func TestCheckAllowed(t *testing.T) {
	m := module + "/internal/"
	input := strings.Join([]string{
		m + "domain/plan context strings " + m + "domain/platform",
		m + "usecase/execute " + m + "domain/plan " + m + "usecase/process io",
		m + "adapter/cli " + m + "adapter/session github.com/spf13/cobra",
		m + "adapter/tui " + m + "adapter/session charm.land/bubbletea/v2",
		module + "/cmd/facman " + m + "adapter/cli " + m + "adapter/tui",
		"",
	}, "\n")
	vs, err := checkReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := report(&out, vs); code != 0 || out.Len() != 0 {
		t.Errorf("exit %d, output %q", code, out.String())
	}
}

func TestCheckDeduplicates(t *testing.T) {
	m := module + "/internal/"
	vs := check(map[string][]string{m + "adapter/tui": {m + "adapter/cli", m + "adapter/cli"}})
	if len(vs) != 1 {
		t.Errorf("violations = %v", vs)
	}
}
