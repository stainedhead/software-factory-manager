package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/stainedhead/software-factory-manager/internal/adapter/session"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/summary"
)

func foundAt(id string, state ownership.State, prov catalog.Provenance, path string) ownership.PackageStatus {
	d := ownership.Detection{PackageID: id, Found: true, Status: ownership.DetectionOK, Provenance: prov, Path: path}
	return ownership.PackageStatus{PackageID: id, State: state, Offered: true, Winner: &d, Detections: []ownership.Detection{d}}
}

func rowFor(t *testing.T, rows []row, id string) row {
	t.Helper()
	for _, r := range rows {
		if r.kind != rowHeader && r.id == id {
			return r
		}
	}
	t.Fatalf("no row %s", id)
	return row{}
}

func TestBuildRowsStates(t *testing.T) {
	cat := fixtureCatalog()
	base := fixtureScan(cat).StatusMap()
	tests := []struct {
		name    string
		with    []string
		edit    func(map[string]ownership.PackageStatus)
		id      string
		state   string
		note    string
		greyed  bool
		section string
	}{
		{name: "OS-bundled foreign", id: "git", state: "present (OS)", edit: func(s map[string]ownership.PackageStatus) {
			st := foundAt("git", ownership.StateForeign, catalog.ProvOS, "/usr/bin/git")
			st.OSBundled = true
			s["git"] = st
		}, section: "foreign"},
		{name: "duplicate names the winner", id: "jq", state: "duplicate", note: "wins on PATH", edit: func(s map[string]ownership.PackageStatus) {
			s["jq"] = foundAt("jq", ownership.StateDuplicate, catalog.ProvBrew, "/opt/homebrew/bin/jq")
		}, section: "duplicate"},
		{name: "drifted", id: "jq", state: "drifted", edit: func(s map[string]ownership.PackageStatus) {
			s["jq"] = ownership.PackageStatus{PackageID: "jq", State: ownership.StateDrifted, Offered: true}
		}, section: "drifted"},
		{name: "adopted", id: "jq", state: "adopted", edit: func(s map[string]ownership.PackageStatus) {
			s["jq"] = foundAt("jq", ownership.StateAdopted, catalog.ProvBrew, "/x")
		}, section: "adopted"},
		{name: "not offered and absent is hidden", id: "", edit: func(s map[string]ownership.PackageStatus) {
			st := s["jq"]
			st.Offered = false
			s["jq"] = st
		}},
		{name: "choice satisfied by a present member", id: "colima", greyed: true, edit: func(s map[string]ownership.PackageStatus) {
			s["orbstack"] = foundAt("orbstack", ownership.StateForeign, catalog.ProvBrewCask, "/Applications/OrbStack.app")
		}},
		{name: "choice member that cannot be picked", id: "colima", greyed: true, note: "needs a VM", edit: func(s map[string]ownership.PackageStatus) {
			s["colima"] = ownership.PackageStatus{PackageID: "colima", State: ownership.StateUnavailableHere, Offered: true, UnavailableReason: "needs a VM"}
		}},
		{name: "helper pulled in by a pick", with: []string{"colima"}, id: "docker", note: "needed by colima"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sts := map[string]ownership.PackageStatus{}
			for k, v := range base {
				sts[k] = v
			}
			if tc.edit != nil {
				tc.edit(sts)
			}
			d, err := catalog.ResolveSelection(cat, catalog.Selection{Profiles: []string{"agentic-factory"}, With: tc.with})
			if err != nil {
				t.Fatal(err)
			}
			rows := buildRows(cat, d, sts)
			if tc.id == "" {
				for _, r := range rows {
					if r.id == "jq" {
						t.Errorf("hidden row shown: %+v", r)
					}
				}
				return
			}
			r := rowFor(t, rows, tc.id)
			if (tc.state != "" && r.state != tc.state) || !strings.Contains(r.note, tc.note) || r.greyed != tc.greyed || r.selectable == tc.greyed && tc.greyed {
				t.Errorf("row = %+v", r)
			}
			if tc.section != "" {
				found := false
				for _, h := range rows {
					found = found || (h.kind == rowHeader && strings.HasPrefix(h.text, tc.section))
				}
				if !found {
					t.Errorf("no %s section", tc.section)
				}
			}
		})
	}
}

func TestChoiceNotes(t *testing.T) {
	cat := fixtureCatalog()
	e, _ := cat.Entry("container-runtime")
	sts := fixtureScan(cat).StatusMap()
	tests := []struct {
		g    catalog.DesiredItem
		sts  map[string]ownership.PackageStatus
		want string
	}{
		{g: catalog.DesiredItem{ID: "container-runtime"}, sts: sts, want: "optional: none picked"},
		{g: catalog.DesiredItem{ID: "container-runtime", Required: true}, sts: sts, want: "required: pick one"},
		{g: catalog.DesiredItem{ID: "container-runtime", ChoicePick: "colima"}, sts: sts, want: "picked colima"},
		{g: catalog.DesiredItem{ID: "container-runtime"}, sts: map[string]ownership.PackageStatus{"colima": foundAt("colima", ownership.StateForeign, catalog.ProvBrew, "/x")}, want: "satisfied by colima"},
	}
	for _, tc := range tests {
		if got := choiceNote(e, tc.g, tc.sts); got != tc.want {
			t.Errorf("choiceNote = %q, want %q", got, tc.want)
		}
	}
	if presentNote(ownership.PackageStatus{}) != "" || conflictNote(ownership.PackageStatus{}) != "" {
		t.Error("notes of nothing")
	}
}

func TestCursorHelpers(t *testing.T) {
	rows := []row{{kind: rowHeader}, {kind: rowItem, id: "a"}, {kind: rowHeader}, {kind: rowItem, id: "b"}}
	tests := []struct{ cur, delta, want int }{
		{-1, +1, 1}, {1, +1, 3}, {3, +1, 3}, {3, -1, 1}, {1, -1, 1}, {0, -1, 1},
	}
	for _, tc := range tests {
		if got := moveCursor(rows, tc.cur, tc.delta); got != tc.want {
			t.Errorf("moveCursor(%d, %d) = %d, want %d", tc.cur, tc.delta, got, tc.want)
		}
	}
	if moveCursor(nil, 0, 1) != 0 {
		t.Error("empty table")
	}
	for _, cur := range []int{-1, 0, 9} {
		if _, ok := cursorRow(rows, cur); ok {
			t.Errorf("cursorRow(%d) found a row", cur)
		}
	}
	if got := filterRows(rows, fixtureCatalog(), "zz"); len(got) != 0 {
		t.Errorf("filter kept %+v", got)
	}
}

func TestMachineText(t *testing.T) {
	tests := []struct {
		name string
		rep  platform.BootstrapReport
		want []string
	}{
		{name: "WSL", rep: platform.BootstrapReport{Platform: platform.Platform{OS: "linux", Arch: "amd64", WSL: true, Distro: platform.Distro{ID: "ubuntu"}}},
			want: []string{"linux/amd64 (WSL), ubuntu", "Homebrew: not installed", "Node:     not found"}},
		{name: "Linux without a display", rep: platform.BootstrapReport{Platform: platform.Platform{OS: "linux", Arch: "amd64"},
			Homebrew: platform.HomebrewInfo{Found: true, Path: "/home/linuxbrew/.linuxbrew/bin/brew"}},
			want: []string{"(no display)", "not on PATH; used through brew shellenv"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := machineText(tc.rep)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("%q misses %q", got, w)
				}
			}
		})
	}
}

func TestSummaryBodies(t *testing.T) {
	tests := []struct {
		name string
		res  session.Result
		err  error
		want []string
	}{
		{name: "nothing ran", want: []string{"Nothing was changed."}},
		{name: "failed before a summary", err: errors.New("installing Homebrew: download failed"), want: []string{"Not everything happened: installing Homebrew"}},
		{name: "nothing to install", res: session.Result{Summary: &summary.Summary{}}, want: []string{"everything wanted is already here"}},
		{name: "profile lines and warnings", res: session.Result{
			Summary: &summary.Summary{ProfileLines: []string{`eval "$(/opt/homebrew/bin/brew shellenv)"`}},
			Outcome: &execute.Outcome{Warnings: []string{"pruning old logs: denied"}}},
			want: []string{"Add to your shell profile:", "brew shellenv", "warning: pruning old logs"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &Model{res: tc.res, err: tc.err, screen: ScreenSummary}
			got := ansi.Strip(m.summaryBody())
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("summary %q misses %q", got, w)
				}
			}
		})
	}
	for s, want := range map[execute.ActionStatus]string{execute.ActionSkipped: "skip", execute.ActionAlreadyPresent: "had", execute.ActionNotStarted: "stop"} {
		if !strings.Contains(resultMark(s), want) {
			t.Errorf("resultMark(%s) = %q", s, resultMark(s))
		}
	}
}

func TestWhyAndSource(t *testing.T) {
	m := &Model{desired: catalog.DesiredSet{Items: []catalog.DesiredItem{
		{ID: "docker", Selected: true, RequiredBy: []string{"colima"}},
		{ID: "jq", Required: true, FromProfiles: []string{"p"}, Version: "1.7"},
		{ID: "gh", FromProfiles: []string{"p"}},
	}}}
	tests := []struct {
		r    row
		want string
	}{
		{row{kind: rowMember, id: "colima", group: "container-runtime"}, "member of the choice group container-runtime"},
		{row{kind: rowItem, id: "docker"}, "needed by colima"},
		{row{kind: rowItem, id: "jq"}, "required by profile p; pinned to 1.7"},
		{row{kind: rowItem, id: "gh"}, "optional in profile p"},
		{row{kind: rowItem, id: "nope"}, "not wanted"},
	}
	for _, tc := range tests {
		if got := m.whyText(tc.r); !strings.Contains(got, tc.want) {
			t.Errorf("whyText(%s) = %q", tc.r.id, got)
		}
	}
	if got := sourceText(catalog.Entry{}, darwin); !strings.Contains(got, "no install step") {
		t.Errorf("sourceText = %q", got)
	}
	if m.wrap("x") != "x" {
		t.Error("wrap without a width")
	}
	if errorText(errors.New("plain")) != "plain" {
		t.Error("errorText of a plain error")
	}
	if got := conflictText(&plan.Conflict{Path: "/p", Owner: plan.OwnerNPM, Evidence: plan.ConflictEvidence{FileType: "Mach-O"}}); !strings.Contains(got, "type:    Mach-O") {
		t.Errorf("conflictText = %q", got)
	}
}

func TestSessionEngineAndProgress(t *testing.T) {
	s := session.New(session.Deps{Bootstrap: failingBoot{}})
	steps, err := (SessionEngine{Session: s}).Start(context.Background(), session.Request{Command: platform.CommandTUIRun})
	if err == nil || steps == nil {
		t.Errorf("Start = %v, %v", steps, err)
	}
	b := newBridge()
	progress{b: b}.CommandOutput(plan.Action{}, "line")
	if msg, ok := (<-b.events).(commandOutputMsg); !ok || msg.line != "line" {
		t.Errorf("msg = %+v", msg)
	}
}

type failingBoot struct{}

func (failingBoot) Check(context.Context) (platform.BootstrapReport, error) {
	return platform.BootstrapReport{}, errors.New("probe failed")
}
