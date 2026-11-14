package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/domain/version"
	"github.com/stainedhead/software-factory-manager/internal/usecase/doctor"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/status"
	"github.com/stainedhead/software-factory-manager/internal/usecase/summary"
)

var t0 = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)

func compile(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(Schema))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("report.v1.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("report.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// validate encodes d and validates it against the embedded schema.
func validate(t *testing.T, s *jsonschema.Schema, d Document) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if err := Encode(&buf, d); err != nil {
		t.Fatal(err)
	}
	return validateBytes(t, s, buf.Bytes())
}

func validateBytes(t *testing.T, s *jsonschema.Schema, b []byte) map[string]any {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(v); err != nil {
		t.Fatalf("schema validation failed: %v\n%s", err, b)
	}
	m, _ := v.(map[string]any)
	return m
}

func fixture() (platform.BootstrapReport, catalog.Catalog, []ownership.PackageStatus) {
	g, _ := version.Parse("2.35")
	clt := true
	boot := platform.BootstrapReport{
		Platform: platform.Platform{OS: "linux", Arch: "amd64", Display: false, Glibc: &g, Distro: platform.Distro{ID: "ubuntu"}, UID: 1000},
		CLT:      &clt,
		Homebrew: platform.HomebrewInfo{Found: true, Path: "/home/linuxbrew/.linuxbrew/bin/brew", Prefix: "/home/linuxbrew/.linuxbrew"},
		Node:     platform.NodeInfo{Found: true, Version: "24.11.0", Provenance: "brew", Satisfies: true, NPM: true},
		Findings: []platform.Finding{{Code: platform.CodeGlibcOld, Severity: platform.SeverityWarning, Message: "glibc 2.35 is old", Fix: "upgrade"}},
	}
	cat := catalog.Catalog{Revision: "embedded:dev", Entries: []catalog.Entry{{ID: "jq", Display: "jq"}, {ID: "claude-code", Display: "Claude Code (CLI)"}, {ID: "gh"}}}
	jq := ownership.Detection{PackageID: "jq", Detector: ownership.DetectorPath, Status: ownership.DetectionOK, Found: true, Path: "/usr/bin/jq", ResolvedPath: "/usr/bin/jq", Provenance: "os", Version: "1.7.1", PathWinner: true}
	statuses := []ownership.PackageStatus{
		{PackageID: "jq", State: ownership.StateForeign, Detections: []ownership.Detection{jq}, Winner: &jq, OSBundled: true, Satisfying: true, Offered: true},
		{PackageID: "claude-code", State: ownership.StateConflict, Offered: true, Conflict: &plan.Conflict{PackageID: "claude-code", Path: "/home/linuxbrew/.linuxbrew/bin/claude",
			Owner: plan.OwnerOrphan, Evidence: plan.ConflictEvidence{LsLine: "lrwxrwxrwx 1 j j 30 Nov 14 19:00 claude -> /nowhere", IsSymlink: true, SymlinkTarget: "/nowhere", Dangling: true, ModTime: t0},
			FailingCommand: []string{"brew", "install", "--cask", "claude-code"}, FixHint: "remove it yourself"}},
		{PackageID: "gh", State: ownership.StateAbsent, Offered: true, Incomplete: true,
			Detections: []ownership.Detection{{PackageID: "gh", Detector: ownership.DetectorBrew, Status: ownership.DetectionUnknown, Reason: "brew info timed out"}}},
	}
	return boot, cat, statuses
}

func TestScanDocument(t *testing.T) {
	s := compile(t)
	boot, cat, statuses := fixture()
	m := validate(t, s, Build(Input{Command: platform.CommandScan, Version: "1.0.0", Now: t0, Bootstrap: &boot, Catalog: &cat, Statuses: statuses,
		Findings: []platform.Finding{{Code: platform.CodeSuperseded, Severity: platform.SeverityInfo, Message: "Gemini CLI found"}}}))
	if m["schema"] != SchemaID || m["command"] != "scan" || m["generated_at"] != "2026-11-14T19:00:00Z" || m["exit_code"] != json.Number("0") && m["exit_code"] != float64(0) {
		t.Errorf("header = %v", m)
	}
	for _, absent := range []string{"plan", "results", "consent_by", "next_steps", "error", "tap_trust"} {
		if _, ok := m[absent]; ok {
			t.Errorf("scan document has %q; optional fields must be omitted", absent)
		}
	}
	pkgs := m["packages"].([]any)
	if len(pkgs) != 3 {
		t.Fatalf("packages = %v", pkgs)
	}
	jq := pkgs[0].(map[string]any)
	if jq["os_bundled"] != true || jq["provenance"] != "os" || jq["version"] != "1.7.1" || jq["path"] != "/usr/bin/jq" {
		t.Errorf("jq = %v", jq)
	}
	if _, ok := jq["selected"]; ok {
		t.Error("scan packages carry no selection")
	}
	cc := pkgs[1].(map[string]any)["conflict"].(map[string]any)
	if cc["owner"] != "orphan" || cc["evidence"].(map[string]any)["dangling"] != true {
		t.Errorf("conflict = %v", cc)
	}
}

func TestPlanApplyDocuments(t *testing.T) {
	s := compile(t)
	boot, cat, statuses := fixture()
	desired := catalog.DesiredSet{Profiles: []string{"agentic-factory"}, Items: []catalog.DesiredItem{{ID: "gh", Required: true, Selected: true}}}
	p := plan.Plan{Profiles: desired.Profiles, CatalogRevision: cat.Revision,
		Actions: []plan.Action{
			{ID: 1, Kind: plan.ActionTap, PackageID: "orca", Commands: []plan.Command{{Argv: []string{"brew", "tap", "stablyai/orca"}, Mutating: true}}, TapTrust: &plan.TapTrustStep{Tap: "stablyai/orca", Item: "stablyai/orca/orca"}},
			{ID: 2, Kind: plan.ActionInstall, PackageID: "gh", Commands: []plan.Command{{Argv: []string{"brew", "install", "gh"}, EnvNames: []string{"PATH"}, Mutating: true}}, DependsOn: []int{1}},
		},
		Excluded: []plan.Exclusion{{PackageID: "claude-code", Reason: plan.ExcludedConflict, Detail: "held"}}}
	in := Input{Command: platform.CommandPlan, Now: t0, Bootstrap: &boot, Catalog: &cat, Desired: &desired, Statuses: statuses, Plan: &p}
	m := validate(t, s, Build(in))
	if _, ok := m["consent_by"]; ok {
		t.Error("plan has no consent_by")
	}
	if got := m["catalog"].(map[string]any)["profiles"].([]any); len(got) != 1 {
		t.Errorf("profiles = %v", got)
	}
	gh := m["packages"].([]any)[2].(map[string]any)
	if gh["selected"] != true || gh["required"] != true || gh["incomplete"] != true {
		t.Errorf("gh = %v", gh)
	}

	// dry run and decline: consent_by is null, no results.
	in.Command = platform.CommandApply
	for _, o := range []*execute.Outcome{nil, {Plan: p, DryRun: true}, {Plan: p, Declined: true}} {
		in.Outcome = o
		m = validate(t, s, Build(in))
		if v, ok := m["consent_by"]; !ok || v != nil {
			t.Errorf("consent_by = %v (present %v), want null", v, ok)
		}
	}

	// a run: consent, results, next steps, an operation error.
	in.Outcome = &execute.Outcome{Plan: p, Consent: plan.NewConsent("j", t0), Results: []execute.ActionResult{
		{ActionID: 1, PackageID: "orca", Status: execute.ActionSucceeded, LogPath: "/s/logs/1.log", OperationID: 1},
		{ActionID: 2, PackageID: "gh", Status: execute.ActionFailed, ExitCode: 1, OperationID: 2, Detail: "brew install gh exited 1"},
	}}
	in.Summary = &summary.Summary{
		Lines:        []summary.Line{{PackageID: "gh", Text: "not installed: failed"}, {PackageID: "claude-code", Installed: true, Text: "installed, sign-in required"}},
		NextSteps:    []string{"Sign in to Claude Code: run claude"},
		ProfileLines: []string{`eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"`, `export PATH="/x/opt/node@24/bin:$PATH"`},
	}
	in.Err = failure.Operation("not everything was installed").WithDetails("action 2 (gh) failed")
	m = validate(t, s, Build(in))
	if m["consent_by"] != "j@2026-11-14T19:00:00Z" || len(m["results"].([]any)) != 2 {
		t.Errorf("apply = %v", m)
	}
	steps := m["next_steps"].([]any)
	if len(steps) != 3 || steps[0].(map[string]any)["package_id"] != "claude-code" || steps[1].(map[string]any)["package_id"] != "homebrew" || steps[2].(map[string]any)["package_id"] != "node" {
		t.Errorf("next_steps = %v", steps)
	}
	e := m["error"].(map[string]any)
	if e["kind"] != "operation" || len(e["details"].([]any)) != 1 {
		t.Errorf("error = %v", e)
	}
	if code := m["exit_code"]; code != json.Number("1") && code != float64(1) {
		t.Errorf("exit_code = %v", code)
	}
}

func TestStatusAndDoctorDocuments(t *testing.T) {
	s := compile(t)
	boot, cat, _ := fixture()
	rec := inventory.Record{ID: "gh", State: inventory.StateManaged, Backend: catalog.BackendBrewFormula, BackendRef: "brew-formula:gh", InstalledAt: t0, InstalledBy: "j@2026-11-14T19:00:00Z"}
	hb := inventory.Record{ID: "homebrew", State: inventory.StateManaged, Backend: inventory.BackendScript, BackendRef: inventory.HomebrewRef, InstalledAt: t0, Bootstrap: true}
	st := status.Report{Rows: []status.Row{{PackageID: "gh", State: ownership.StateManaged, Provenance: "brew", Path: "/p/bin/gh", Version: "2.1.0", Record: &rec}},
		Homebrew: &hb, TapTrust: []inventory.TapTrustGrant{{PackageID: "orca", Tap: "stablyai/orca", Item: "stablyai/orca/orca", GrantedAt: t0, OperationID: 3}}}
	m := validate(t, s, Build(Input{Command: platform.CommandStatus, Now: t0, Bootstrap: &boot, Catalog: &cat, Status: &st}))
	pkgs := m["packages"].([]any)
	if len(pkgs) != 2 || pkgs[1].(map[string]any)["record"].(map[string]any)["bootstrap"] != true || len(m["tap_trust"].([]any)) != 1 {
		t.Errorf("status = %v", m)
	}

	dr := doctor.Report{Findings: []platform.Finding{
		{Code: platform.CodeConflict, Severity: platform.SeverityWarning, Message: "claude-code: /p/bin/claude is held by orphan", Fix: "remove it"},
		{Code: platform.CodeUntrustedTap, Severity: platform.SeverityWarning, Message: "orca: cask stablyai/orca/orca is not trusted", Fix: "brew trust --cask stablyai/orca/orca"},
		{Code: platform.CodeGlibcOld, Severity: platform.SeverityWarning, Message: "glibc: 2.35 is old"},
	}}
	m = validate(t, s, Build(Input{Command: platform.CommandDoctor, Now: t0, Bootstrap: &boot, Catalog: &cat, Doctor: &dr, Err: dr.Err()}))
	fs := m["findings"].([]any)
	if fs[0].(map[string]any)["package_id"] != "claude-code" || fs[1].(map[string]any)["package_id"] != "" && fs[1].(map[string]any)["package_id"] != nil {
		// orca is not in this small catalog, so it gets no package_id
		t.Errorf("findings = %v", fs)
	}
	if _, ok := fs[2].(map[string]any)["package_id"]; ok {
		t.Error("a non-package finding has a package_id")
	}
	if _, ok := m["packages"]; ok {
		t.Error("doctor lists findings, not packages")
	}
	if code := m["exit_code"]; code != json.Number("2") && code != float64(2) {
		t.Errorf("exit_code = %v", code)
	}
}

func TestMinimalAndBrokenDocuments(t *testing.T) {
	s := compile(t)
	// a bootstrap failure before anything was found: header, error, exit 4.
	m := validate(t, s, Build(Input{Command: platform.CommandScan, Err: failure.Bootstrap("probing the platform: %w", errors.New("boom")).WithFix("fix it")}))
	if m["error"].(map[string]any)["kind"] != "bootstrap" || m["facman"].(map[string]any)["version"] != "dev" {
		t.Errorf("minimal = %v", m)
	}
	// an untyped error is an operation error.
	if e := errorOf(errors.New("x")); e.Kind != "operation" {
		t.Errorf("errorOf = %+v", e)
	}

	broken := []string{
		`{"schema":"facman.report/v2","command":"scan","facman":{"name":"facman","title":"t","version":"1"},"generated_at":"2026-11-14T19:00:00Z","exit_code":0}`,
		`{"schema":"facman.report/v1","command":"scan","facman":{"name":"facman","title":"t","version":"1"},"generated_at":"2026-11-14T19:00:00Z","exit_code":5}`,
		`{"schema":"facman.report/v1","command":"scan","facman":{"name":"facman","title":"t","version":"1"},"generated_at":"2026-11-14T19:00:00Z","exit_code":0,"packages":[{"id":"jq","state":"weird"}]}`,
		`{"schema":"facman.report/v1","command":"scan","facman":{"name":"facman","title":"t","version":"1"},"generated_at":"2026-11-14T19:00:00Z","exit_code":0,"extra":1}`,
		`{"schema":"facman.report/v1","command":"apply","facman":{"name":"facman","title":"t","version":"1"},"generated_at":"2026-11-14T19:00:00Z","exit_code":0,"consent_by":"nobody"}`,
	}
	for _, b := range broken {
		v, err := jsonschema.UnmarshalJSON(strings.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Validate(v); err == nil {
			t.Errorf("schema accepted a broken document: %s", b)
		}
	}
}

func TestNullableAndEncodeErrors(t *testing.T) {
	v := "x"
	b, err := Nullable{Value: &v}.MarshalJSON()
	if err != nil || string(b) != `"x"` {
		t.Errorf("Nullable = %s, %v", b, err)
	}
	if err := Encode(failWriter{}, Document{}); err == nil {
		t.Error("want a write error")
	}
	if got := stamp(time.Time{}); got == "" {
		t.Error("zero time gets the current time")
	}
	if c := conflictOf(&plan.Conflict{Path: "/x", Owner: plan.OwnerOrphan}); c.FailingCommand[0] != "unknown" || c.FixHint == "" {
		t.Errorf("conflict defaults = %+v", c)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }
