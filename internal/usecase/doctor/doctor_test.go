package doctor

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

type tapState struct {
	tapped, trusted bool
	err             error
}

type fakeTaps map[string]tapState

func (f fakeTaps) TapState(_ context.Context, s catalog.InstallStep) (bool, bool, error) {
	st := f[s.Tap]
	return st.tapped, st.trusted, st.err
}

type fakeConflicts struct {
	conflicts map[string]*plan.Conflict
	errs      map[string]error
	checked   *[]string
}

func (f fakeConflicts) Check(_ context.Context, e catalog.Entry, _ catalog.InstallStep) (*plan.Conflict, error) {
	if f.checked != nil {
		*f.checked = append(*f.checked, e.ID)
	}
	return f.conflicts[e.ID], f.errs[e.ID]
}

var darwin = platform.Platform{OS: "darwin", Display: true}

func on(b catalog.BackendID, id string) map[platform.Key][]catalog.InstallStep {
	return map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: b, ID: id}}}
}

func testCatalog() catalog.Catalog {
	orca := catalog.Entry{ID: "orca", Kind: catalog.KindGUI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {
		{Backend: catalog.BackendBrewCask, ID: "stablyai/orca/orca", Tap: "stablyai/orca", Trust: catalog.TrustItem}}}}
	tool := catalog.Entry{ID: "tool", Kind: catalog.KindCLI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {
		{Backend: catalog.BackendBrewFormula, ID: "acme/tools/tool", Tap: "acme/tools"}}}}
	return catalog.Catalog{Entries: []catalog.Entry{
		{ID: "claude-code", Kind: catalog.KindCLI, Install: on(catalog.BackendBrewCask, "claude-code")},
		orca,
		tool,
		{ID: "gh", Kind: catalog.KindCLI, Install: on(catalog.BackendBrewFormula, "gh")},
		{ID: "paperclip", Kind: catalog.KindCLI, Install: on(catalog.BackendNPM, "paperclipai")},
		{ID: "linux-only", Kind: catalog.KindCLI, Install: map[platform.Key][]catalog.InstallStep{platform.Linux: {{Backend: catalog.BackendBrewFormula, ID: "x"}}}},
		{ID: "gemini-cli", Kind: catalog.KindCLI, SupersededBy: "claude-code"},
		{ID: "runtime", Choice: []catalog.ChoiceMember{{ID: "gh"}}},
	}}
}

func service(taps fakeTaps, c fakeConflicts) *Service {
	return &Service{Taps: taps, Conflicts: c, Implemented: catalog.BackendSet{catalog.BackendBrewCask: true, catalog.BackendBrewFormula: true}}
}

func codes(r Report) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.Code+":"+string(f.Severity))
	}
	return out
}

func TestRun(t *testing.T) {
	trusted := fakeTaps{"stablyai/orca": {tapped: true, trusted: true}, "acme/tools": {tapped: true, trusted: false}}
	brewInstalled := map[string]ownership.PackageStatus{"orca": {Detections: []ownership.Detection{{Found: true, BackendRef: "brew-cask:stablyai/orca/orca"}}}}
	tests := []struct {
		name     string
		taps     fakeTaps
		conf     fakeConflicts
		statuses map[string]ownership.PackageStatus
		findings []platform.Finding
		codes    []string
		exit     int
	}{
		{"clean", trusted, fakeConflicts{}, nil, nil, nil, 0},
		{"info only", fakeTaps{"acme/tools": {tapped: true, trusted: false}}, fakeConflicts{errs: map[string]error{"gh": errors.New("brew info: exit 1")}},
			nil, []platform.Finding{{Code: platform.CodeSuperseded, Severity: platform.SeverityInfo}},
			[]string{"superseded:info", "tap-not-added:info", "check-failed:info"}, 0},
		{"conflict", trusted, fakeConflicts{conflicts: map[string]*plan.Conflict{"claude-code": {
			PackageID: "claude-code", Path: "/opt/homebrew/bin/claude", Owner: plan.OwnerNPM, OwnerDetail: "npm @anthropic-ai/claude-code",
			FailingCommand: []string{"brew", "install", "--cask", "claude-code"}, FixHint: "npm uninstall -g @anthropic-ai/claude-code"}}},
			nil, nil, []string{"conflict:warning"}, 2},
		{"tap added, item untrusted", fakeTaps{"stablyai/orca": {tapped: true, trusted: false}, "acme/tools": {tapped: true, trusted: false}}, fakeConflicts{}, nil, nil, []string{"untrusted-tap:warning"}, 2},
		{"installed through brew, untrusted", fakeTaps{"acme/tools": {tapped: true, trusted: false}}, fakeConflicts{}, brewInstalled, nil, []string{"untrusted-tap:warning"}, 2},
		{"tap state unreadable", fakeTaps{"stablyai/orca": {err: errors.New("brew tap: exit 1")}, "acme/tools": {tapped: true, trusted: false}}, fakeConflicts{}, nil, nil, []string{"check-failed:info"}, 0},
		{"bootstrap warning", trusted, fakeConflicts{}, nil, []platform.Finding{{Code: platform.CodeGlibcOld, Severity: platform.SeverityWarning}}, []string{"glibc-old:warning"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var checked []string
			tt.conf.checked = &checked
			r := service(tt.taps, tt.conf).Run(context.Background(), Input{Catalog: testCatalog(), Platform: darwin, Statuses: tt.statuses, Findings: tt.findings})
			if got := codes(r); !slices.Equal(got, tt.codes) {
				t.Errorf("findings = %v, want %v", got, tt.codes)
			}
			if got := failure.ExitCode(r.Err()); got != tt.exit {
				t.Errorf("exit = %d, want %d", got, tt.exit)
			}
			// every brew item available here is checked, not only selected ones.
			if want := []string{"claude-code", "orca", "tool", "gh"}; !slices.Equal(checked, want) || !slices.Equal(r.Checked, want) {
				t.Errorf("checked %v / %v", checked, r.Checked)
			}
		})
	}
}

func TestFindingText(t *testing.T) {
	r := service(fakeTaps{"stablyai/orca": {tapped: true, trusted: false}, "acme/tools": {}}, fakeConflicts{conflicts: map[string]*plan.Conflict{"gh": {
		PackageID: "gh", Path: "/opt/homebrew/bin/gh", Owner: plan.OwnerOtherBrew, OwnerDetail: "brew formula gh-legacy",
		FailingCommand: []string{"brew", "install", "gh"}, FixHint: "uninstall gh-legacy"}}}).
		Run(context.Background(), Input{Catalog: testCatalog(), Platform: darwin})
	if len(r.Findings) != 3 || len(r.Conflicts) != 1 {
		t.Fatalf("report = %+v", r)
	}
	untrusted, pending, conflict := r.Findings[0], r.Findings[1], r.Findings[2]
	if untrusted.Fix != "brew trust --cask stablyai/orca/orca" || !strings.Contains(untrusted.Message, "not trusted") {
		t.Errorf("untrusted = %+v", untrusted)
	}
	if !strings.Contains(pending.Message, "acme/tools is not added") || strings.Contains(pending.Message, "trusts") {
		t.Errorf("pending = %+v", pending)
	}
	if conflict.Fix != "uninstall gh-legacy" || !strings.Contains(conflict.Message, "brew install gh would fail") {
		t.Errorf("conflict = %+v", conflict)
	}
	formula := service(fakeTaps{"stablyai/orca": {}, "acme/tools": {tapped: true, trusted: false}}, fakeConflicts{}).Run(context.Background(), Input{Catalog: catalog.Catalog{Entries: []catalog.Entry{{
		ID: "f", Kind: catalog.KindCLI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewFormula, ID: "acme/tools/f", Tap: "acme/tools", Trust: catalog.TrustItem}}}}}}, Platform: darwin})
	if len(formula.Findings) != 1 || formula.Findings[0].Fix != "brew trust --formula acme/tools/f" {
		t.Errorf("formula = %+v", formula.Findings)
	}
	none := service(fakeTaps{"stablyai/orca": {}}, fakeConflicts{}).Run(context.Background(), Input{Catalog: catalog.Catalog{Entries: testCatalog().Entries[1:2]}, Platform: darwin})
	if len(none.Findings) != 1 || !strings.Contains(none.Findings[0].Message, "and trusts stablyai/orca/orca") {
		t.Errorf("not tapped = %+v", none.Findings)
	}
}
