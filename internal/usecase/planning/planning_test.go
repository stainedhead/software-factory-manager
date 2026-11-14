package planning

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

// brewLike behaves like adapter/brew: tap and trust actions for tap items
// unless already in place, then the install.
type brewLike struct {
	id              catalog.BackendID
	tapped, trusted bool
	err             error
	forbidden       bool
}

func (b brewLike) ID() catalog.BackendID        { return b.id }
func (brewLike) Available(context.Context) bool { return true }
func (b brewLike) InstallActions(_ context.Context, e catalog.Entry, s catalog.InstallStep) ([]plan.Action, error) {
	if b.err != nil {
		return nil, b.err
	}
	var out []plan.Action
	tt := &plan.TapTrustStep{Tap: s.Tap, ItemKind: plan.ItemCask, Item: s.ID, PackageID: e.ID}
	if s.Tap != "" && !b.tapped {
		out = append(out, plan.Action{Kind: plan.ActionTap, PackageID: e.ID, TapTrust: tt, Commands: []plan.Command{{Argv: []string{"brew", "tap", s.Tap}, Mutating: true}}})
	}
	if s.Tap != "" && !b.trusted {
		out = append(out, plan.Action{Kind: plan.ActionTrust, PackageID: e.ID, TapTrust: tt, Commands: []plan.Command{{Argv: []string{"brew", "trust", "--cask", s.ID}, Mutating: true}}})
	}
	argv := []string{"brew", "install", s.ID}
	if b.id == catalog.BackendBrewCask {
		argv = []string{"brew", "install", "--cask", s.ID}
	}
	if b.forbidden {
		argv = append(argv, "--force")
	}
	return append(out, plan.Action{Kind: plan.ActionInstall, PackageID: e.ID, Commands: []plan.Command{{Argv: argv, Mutating: true}}}), nil
}
func (brewLike) LinkPaths(context.Context, catalog.Entry, catalog.InstallStep) ([]string, error) {
	return nil, nil
}
func (b brewLike) TapState(context.Context, catalog.InstallStep) (bool, bool, error) {
	return b.tapped, b.trusted, nil
}
func (brewLike) Capabilities() Caps { return Caps{} }

func onDarwin(b catalog.BackendID, id string) map[platform.Key][]catalog.InstallStep {
	return map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: b, ID: id}}}
}

func cli(id string, b catalog.BackendID, requires ...string) catalog.Entry {
	e := catalog.Entry{ID: id, Kind: catalog.KindCLI, Install: onDarwin(b, id)}
	for _, r := range requires {
		e.Requires = append(e.Requires, catalog.Requirement{Package: r})
	}
	return e
}

func testCatalog() catalog.Catalog {
	orca := catalog.Entry{ID: "orca", Kind: catalog.KindGUI, Install: map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: catalog.BackendBrewCask, ID: "stablyai/orca/orca", Tap: "stablyai/orca", Trust: catalog.TrustItem}}}}
	return catalog.Catalog{APIVersion: catalog.APIVersion, Revision: "embedded:test", Entries: []catalog.Entry{
		cli("claude-code", catalog.BackendBrewCask),
		orca,
		cli("paperclip", catalog.BackendNPM),
		cli("jq", catalog.BackendBrewFormula),
		cli("node", catalog.BackendBrewFormula),
		cli("awscli", catalog.BackendBrewFormula),
		{ID: "container-runtime", Choice: []catalog.ChoiceMember{{ID: "orbstack"}, {ID: "colima"}}, SatisfiedByAnyPresent: true},
		{ID: "orbstack", Kind: catalog.KindGUI, Install: onDarwin(catalog.BackendBrewCask, "orbstack")},
		cli("colima", catalog.BackendBrewFormula, "docker"),
		cli("docker", catalog.BackendBrewFormula),
		cli("session-manager-plugin", catalog.BackendBrewCask, "awscli"),
		cli("t3-code", catalog.BackendBrewCask, "claude-code"),
		{ID: "linux-only", Kind: catalog.KindCLI, Install: map[platform.Key][]catalog.InstallStep{platform.Linux: {{Backend: catalog.BackendBrewFormula, ID: "x"}}}},
	}, Profiles: map[string]catalog.Profile{
		"p": {Name: "p", Packages: []catalog.ProfilePackage{{ID: "claude-code"}, {ID: "orca"}, {ID: "paperclip"}, {ID: "jq"}, {ID: "awscli"},
			{ID: "container-runtime", Optional: true}, {ID: "session-manager-plugin", Optional: true}, {ID: "t3-code", Optional: true}, {ID: "linux-only"}}},
	}}
}

var (
	darwinPlat = platform.Platform{OS: "darwin", Display: true}
	brewFound  = platform.BootstrapReport{Platform: darwinPlat, Homebrew: platform.HomebrewInfo{Found: true, Prefix: "/opt/homebrew", OnPATH: true}}
	installer  = plan.Command{Argv: []string{"/bin/bash", "<downloaded Homebrew install.sh>"}, EnvNames: []string{"NONINTERACTIVE"}}
)

func statuses(c catalog.Catalog, p platform.Platform, override map[string]ownership.PackageStatus) map[string]ownership.PackageStatus {
	impl := catalog.BackendSet{catalog.BackendBrewFormula: true, catalog.BackendBrewCask: true}
	out := map[string]ownership.PackageStatus{}
	for _, e := range c.Entries {
		out[e.ID] = ownership.Classify(e, nil, nil, nil, catalog.SelectInstall(e, p, impl))
	}
	for id, st := range override {
		st.PackageID = id
		st.Offered = true
		out[id] = st
	}
	return out
}

func present(prov catalog.Provenance) ownership.PackageStatus {
	d := ownership.Detection{Found: true, Provenance: prov, Path: "/x", Status: ownership.DetectionOK}
	return ownership.PackageStatus{State: ownership.StateForeign, Winner: &d, Detections: []ownership.Detection{d}}
}

func svc(b brewLike) *Service {
	cask, formula := b, b
	cask.id, formula.id = catalog.BackendBrewCask, catalog.BackendBrewFormula
	return &Service{Backends: map[catalog.BackendID]Backend{catalog.BackendBrewCask: cask, catalog.BackendBrewFormula: formula}, InstallerCommand: installer}
}

type step struct {
	kind plan.ActionKind
	pkg  string
	argv string
	deps []int
}

func summarize(p plan.Plan) []step {
	var out []step
	for _, a := range p.Actions {
		var argv []string
		for _, c := range a.Commands {
			argv = append(argv, strings.Join(c.Argv, " "))
		}
		out = append(out, step{a.Kind, a.PackageID, strings.Join(argv, "; "), a.DependsOn})
	}
	return out
}

func exclusions(p plan.Plan) map[string]plan.ExclusionReason {
	m := map[string]plan.ExclusionReason{}
	for _, e := range p.Excluded {
		m[e.PackageID] = e.Reason
	}
	return m
}

func TestBuild(t *testing.T) {
	c := testCatalog()
	unknown := ownership.Detection{Detector: ownership.DetectorBrew, Status: ownership.DetectionUnknown, Reason: "brew detector timed out after 10s"}
	conflict := &plan.Conflict{PackageID: "claude-code", Path: "/opt/homebrew/bin/claude", Owner: plan.OwnerOrphan, OwnerDetail: "dangling symlink", FixHint: "remove it"}
	tests := []struct {
		name      string
		with      []string
		boot      platform.BootstrapReport
		backend   brewLike
		override  map[string]ownership.PackageStatus
		want      []step
		excluded  map[string]plan.ExclusionReason
		detailHas map[string]string
	}{
		{name: "everything absent, brew found", boot: brewFound,
			want: []step{
				{plan.ActionInstall, "claude-code", "brew install --cask claude-code", nil},
				{plan.ActionTap, "orca", "brew tap stablyai/orca", nil},
				{plan.ActionTrust, "orca", "brew trust --cask stablyai/orca/orca", []int{2}},
				{plan.ActionInstall, "orca", "brew install --cask stablyai/orca/orca", []int{3}},
				{plan.ActionInstall, "jq", "brew install jq", nil},
				{plan.ActionInstall, "awscli", "brew install awscli", nil},
			},
			excluded: map[string]plan.ExclusionReason{"paperclip": plan.ExcludedUnavailableHere}},
		{name: "already tapped and trusted", boot: brewFound, backend: brewLike{tapped: true, trusted: true},
			override: map[string]ownership.PackageStatus{"claude-code": present(catalog.ProvNative), "jq": present(catalog.ProvBrew), "awscli": present(catalog.ProvBrew)},
			want:     []step{{plan.ActionInstall, "orca", "brew install --cask stablyai/orca/orca", nil}}},
		{name: "brew missing: installer first, everything depends on it",
			boot:     platform.BootstrapReport{Platform: darwinPlat, Findings: []platform.Finding{{Code: platform.CodeBrewMissing, Severity: platform.SeverityWarning}, {Code: platform.CodeNoDisplay, Severity: platform.SeverityInfo}}},
			override: map[string]ownership.PackageStatus{"claude-code": present(catalog.ProvNative), "orca": present(catalog.ProvAppBundle), "awscli": present(catalog.ProvBrew)},
			want: []step{
				{plan.ActionBootstrapHomebrew, "homebrew", "/bin/bash <downloaded Homebrew install.sh>", nil},
				{plan.ActionInstall, "jq", "brew install jq", []int{1}},
			}},
		{name: "dependencies first", boot: brewFound, with: []string{"session-manager-plugin"},
			override: map[string]ownership.PackageStatus{"claude-code": present(catalog.ProvNative), "orca": present(catalog.ProvAppBundle), "jq": present(catalog.ProvBrew)},
			want: []step{
				{plan.ActionInstall, "awscli", "brew install awscli", nil},
				{plan.ActionInstall, "session-manager-plugin", "brew install --cask session-manager-plugin", []int{1}},
			}},
		{name: "conflict excludes the package and its dependents", boot: brewFound, with: []string{"t3-code"},
			override:  map[string]ownership.PackageStatus{"claude-code": {State: ownership.StateConflict, Conflict: conflict}, "orca": present(catalog.ProvAppBundle), "jq": present(catalog.ProvBrew), "awscli": present(catalog.ProvBrew)},
			excluded:  map[string]plan.ExclusionReason{"claude-code": plan.ExcludedConflict, "t3-code": plan.ExcludedDependency, "paperclip": plan.ExcludedUnavailableHere},
			detailHas: map[string]string{"claude-code": "/opt/homebrew/bin/claude is held by orphan", "t3-code": "depends on claude-code (conflict)"}},
		{name: "detection incomplete is never planned", boot: brewFound,
			override:  map[string]ownership.PackageStatus{"claude-code": {State: ownership.StateAbsent, Incomplete: true, Detections: []ownership.Detection{unknown, unknown}}, "orca": present(catalog.ProvAppBundle), "jq": present(catalog.ProvBrew), "awscli": present(catalog.ProvBrew)},
			excluded:  map[string]plan.ExclusionReason{"claude-code": plan.ExcludedDetectionIncomplete, "paperclip": plan.ExcludedUnavailableHere},
			detailHas: map[string]string{"claude-code": "detection incomplete: brew detector timed out after 10s"}},
		{name: "present in any state gets no action", boot: brewFound,
			override: map[string]ownership.PackageStatus{
				"claude-code": {State: ownership.StateDuplicate}, "orca": {State: ownership.StateManaged},
				"jq": {State: ownership.StateDrifted}, "awscli": {State: ownership.StateAdopted}},
			excluded: map[string]plan.ExclusionReason{"paperclip": plan.ExcludedUnavailableHere}},
		{name: "choice pick installed with its helper", boot: brewFound, with: []string{"colima"},
			override: map[string]ownership.PackageStatus{"claude-code": present(catalog.ProvNative), "orca": present(catalog.ProvAppBundle), "jq": present(catalog.ProvBrew), "awscli": present(catalog.ProvBrew)},
			want: []step{
				{plan.ActionInstall, "docker", "brew install docker", nil},
				{plan.ActionInstall, "colima", "brew install colima", []int{1}},
			}},
		{name: "choice group satisfied by any present member", boot: brewFound, with: []string{"colima"},
			override: map[string]ownership.PackageStatus{"orbstack": present(catalog.ProvAppBundle), "claude-code": present(catalog.ProvNative), "orca": present(catalog.ProvAppBundle), "jq": present(catalog.ProvBrew), "awscli": present(catalog.ProvBrew)}},
		{name: "empty plan, no installer", boot: platform.BootstrapReport{Platform: darwinPlat},
			override: map[string]ownership.PackageStatus{"claude-code": present(catalog.ProvNative), "orca": present(catalog.ProvAppBundle), "jq": present(catalog.ProvOS), "awscli": present(catalog.ProvBrew)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := catalog.Resolve(c, []string{"p"}, tt.with)
			if err != nil {
				t.Fatal(err)
			}
			in := Input{Catalog: c, Desired: d, Statuses: statuses(c, tt.boot.Platform, tt.override), Bootstrap: tt.boot}
			p, err := svc(tt.backend).Build(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			got := summarize(p)
			if !slices.EqualFunc(got, tt.want, func(a, b step) bool {
				return a.kind == b.kind && a.pkg == b.pkg && a.argv == b.argv && slices.Equal(a.deps, b.deps)
			}) {
				t.Errorf("actions:\n got %+v\nwant %+v", got, tt.want)
			}
			wantEx := tt.excluded
			if wantEx == nil {
				wantEx = map[string]plan.ExclusionReason{"paperclip": plan.ExcludedUnavailableHere}
			}
			if got := exclusions(p); len(got) != len(wantEx) {
				t.Errorf("excluded = %v, want %v", got, wantEx)
			} else {
				for id, r := range wantEx {
					if got[id] != r {
						t.Errorf("excluded %s = %s, want %s", id, got[id], r)
					}
				}
			}
			for id, part := range tt.detailHas {
				for _, ex := range p.Excluded {
					if ex.PackageID == id && !strings.Contains(ex.Detail, part) {
						t.Errorf("%s detail = %q, want %q", id, ex.Detail, part)
					}
				}
			}
			if p.CatalogRevision != "embedded:test" || !slices.Equal(p.Profiles, []string{"p"}) {
				t.Errorf("plan meta = %+v", p)
			}
			if p.Empty() != (len(tt.want) == 0) {
				t.Error("Empty")
			}
		})
	}
}

func TestBuildInstallerAndWarnings(t *testing.T) {
	c := testCatalog()
	boot := platform.BootstrapReport{Platform: darwinPlat, Findings: []platform.Finding{
		{Code: platform.CodeBrewMissing, Severity: platform.SeverityWarning}, {Code: platform.CodeNodeOld, Severity: platform.SeverityInfo}}}
	d, _ := catalog.Resolve(c, []string{"p"}, nil)
	p, err := svc(brewLike{}).Build(context.Background(), Input{Catalog: c, Desired: d, Statuses: statuses(c, darwinPlat, nil), Bootstrap: boot})
	if err != nil {
		t.Fatal(err)
	}
	inst := p.Actions[0].Commands[0]
	if !inst.NeedsSudo || !inst.Interactive || !inst.Unverified || !inst.Mutating || inst.Argv[0] != "/bin/bash" {
		t.Errorf("installer = %+v", inst)
	}
	for _, a := range p.Actions[1:] {
		if !slices.Contains(a.DependsOn, 1) {
			t.Errorf("action %d does not depend on the installer", a.ID)
		}
	}
	if len(p.Warnings) != 1 || p.Warnings[0].Code != platform.CodeBrewMissing {
		t.Errorf("warnings = %+v", p.Warnings)
	}
}

func TestBuildChoiceWithoutPick(t *testing.T) {
	c := testCatalog()
	prof := c.Profiles["p"]
	prof.Packages = []catalog.ProfilePackage{{ID: "container-runtime"}, {ID: "linux-only"}}
	c.Profiles["p"] = prof
	d, _ := catalog.Resolve(c, []string{"p"}, nil)
	p, err := svc(brewLike{}).Build(context.Background(), Input{Catalog: c, Desired: d, Statuses: statuses(c, darwinPlat, nil), Bootstrap: brewFound})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Empty() || exclusions(p)["container-runtime"] != plan.ExcludedNotSelected || !strings.Contains(p.Excluded[0].Detail, "orbstack, colima") {
		t.Errorf("plan = %+v", p)
	}
}

func TestBuildDeselectedRequired(t *testing.T) {
	c := testCatalog()
	tests := []struct {
		name     string
		override map[string]ownership.PackageStatus
		want     bool
	}{
		{name: "absent and unticked is reported not selected", want: true},
		{name: "present and unticked is not reported", override: map[string]ownership.PackageStatus{"jq": present(catalog.ProvBrew)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := catalog.ResolveSelection(c, catalog.Selection{Profiles: []string{"p"}, Without: []string{"jq"}})
			if err != nil {
				t.Fatal(err)
			}
			p, err := svc(brewLike{}).Build(context.Background(), Input{Catalog: c, Desired: d, Statuses: statuses(c, darwinPlat, tc.override), Bootstrap: brewFound})
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range p.Actions {
				if a.PackageID == "jq" {
					t.Errorf("jq planned: %+v", a)
				}
			}
			if got := exclusions(p)["jq"] == plan.ExcludedNotSelected; got != tc.want {
				t.Errorf("jq not-selected = %v, want %v (%+v)", got, tc.want, p.Excluded)
			}
		})
	}
}

func TestBuildErrors(t *testing.T) {
	c := testCatalog()
	d, _ := catalog.Resolve(c, []string{"p"}, nil)
	in := Input{Catalog: c, Desired: d, Statuses: statuses(c, darwinPlat, nil), Bootstrap: brewFound}
	if _, err := svc(brewLike{err: errors.New("boom")}).Build(context.Background(), in); err == nil {
		t.Error("backend error must be returned")
	}
	_, err := svc(brewLike{forbidden: true}).Build(context.Background(), in)
	if failure.ExitCode(err) != 1 || !errors.Is(err, plan.ErrInvariant) {
		t.Errorf("invariant violation = %v", err)
	}
	cyc := testCatalog()
	cyc.Entries[3].Requires = []catalog.Requirement{{Package: "awscli"}}
	cyc.Entries[5].Requires = []catalog.Requirement{{Package: "jq"}}
	in.Catalog = cyc
	if _, err := svc(brewLike{}).Build(context.Background(), in); failure.ExitCode(err) != 3 {
		t.Errorf("cycle = %v", err)
	}
	// a selected package whose platform step needs a backend this service lacks.
	only := &Service{Backends: map[catalog.BackendID]Backend{catalog.BackendBrewFormula: brewLike{id: catalog.BackendBrewFormula}}}
	in.Catalog = c
	in.Statuses = statuses(c, darwinPlat, nil)
	p, err := only.Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if exclusions(p)["claude-code"] != plan.ExcludedUnavailableHere {
		t.Errorf("excluded = %v", exclusions(p))
	}
}
