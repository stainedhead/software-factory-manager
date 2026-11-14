package scan

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

var (
	t0       = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)
	darwin   = platform.Platform{OS: "darwin", Display: true}
	backends = catalog.BackendSet{catalog.BackendBrewFormula: true, catalog.BackendBrewCask: true}
)

func steps(b catalog.BackendID, id string) map[platform.Key][]catalog.InstallStep {
	return map[platform.Key][]catalog.InstallStep{platform.Darwin: {{Backend: b, ID: id}}}
}

func testCatalog() catalog.Catalog {
	return catalog.Catalog{APIVersion: catalog.APIVersion, Revision: "embedded:test", Entries: []catalog.Entry{
		{ID: "jq", Kind: catalog.KindCLI, Detect: catalog.DetectSpec{Commands: []string{"jq"}}, Install: steps(catalog.BackendBrewFormula, "jq")},
		{ID: "gh", Kind: catalog.KindCLI, Detect: catalog.DetectSpec{Commands: []string{"gh"}}, Install: steps(catalog.BackendBrewFormula, "gh")},
		{ID: "iterm2", Kind: catalog.KindGUI, Detect: catalog.DetectSpec{BundleIDs: []string{"com.googlecode.iterm2"}}, Install: steps(catalog.BackendBrewCask, "iterm2")},
		{ID: "paperclip", Kind: catalog.KindCLI, Detect: catalog.DetectSpec{Commands: []string{"paperclipai"}, NPMPackage: "paperclipai"}, Install: steps(catalog.BackendNPM, "paperclipai")},
		{ID: "antigravity-cli", Display: "Google Antigravity CLI (agy)", Kind: catalog.KindCLI, Detect: catalog.DetectSpec{Commands: []string{"agy"}}, Install: steps(catalog.BackendBrewCask, "antigravity-cli")},
		{ID: "gemini-cli", Display: "Gemini CLI", Kind: catalog.KindCLI, SupersededBy: "antigravity-cli", Detect: catalog.DetectSpec{Commands: []string{"gemini"}, NPMPackage: "@google/gemini-cli"}},
	}}
}

// fakeDet returns canned detections, or blocks, or fails.
type fakeDet struct {
	id      ownership.DetectorID
	dets    []ownership.Detection
	err     error
	block   bool
	started *sync.WaitGroup
	all     chan struct{}
	mu      sync.Mutex
	got     []string
}

func (f *fakeDet) ID() ownership.DetectorID { return f.id }
func (f *fakeDet) Detect(ctx context.Context, entries []catalog.Entry, _ Env) ([]ownership.Detection, error) {
	f.mu.Lock()
	for _, e := range entries {
		f.got = append(f.got, e.ID)
	}
	f.mu.Unlock()
	if f.started != nil {
		f.started.Done()
		<-f.all
	}
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.dets, f.err
}

type memInventory struct {
	records     []inventory.Record
	interrupted []inventory.Operation
	upserts     []inventory.Record
	snaps       []inventory.DetectionSnapshot
	marked      bool
	err         error
}

func (m *memInventory) Records(context.Context) ([]inventory.Record, error) {
	return append(slices.Clone(m.records), m.upserts...), m.err
}
func (m *memInventory) UpsertRecord(_ context.Context, r inventory.Record) error {
	m.upserts = append(m.upserts, r)
	return nil
}
func (m *memInventory) MarkInterrupted(context.Context) ([]inventory.Operation, error) {
	m.marked = true
	return m.interrupted, nil
}
func (m *memInventory) SaveDetections(_ context.Context, s []inventory.DetectionSnapshot) error {
	m.snaps = append(m.snaps, s...)
	return nil
}

type lock struct {
	holder   int
	held     bool // held by a process whose PID could not be read
	err      error
	unlocked bool
}

func (l *lock) TryLock() (func(), int, error) {
	if l.err != nil {
		return nil, 0, l.err
	}
	if l.holder != 0 || l.held {
		return nil, l.holder, nil
	}
	return func() { l.unlocked = true }, 0, nil
}

func found(det ownership.DetectorID, pkg string, prov catalog.Provenance, path, ref string) ownership.Detection {
	return ownership.Detection{PackageID: pkg, Detector: det, Status: ownership.DetectionOK, Found: true, Path: path, ResolvedPath: path, Provenance: prov, BackendRef: ref}
}

func newService(inv *memInventory, l *lock, dets ...Detector) *Service {
	return &Service{Detectors: dets, Inventory: inv, State: l, Now: func() time.Time { return t0 }, Implemented: backends}
}

func TestScanClassifiesEveryEntry(t *testing.T) {
	path := &fakeDet{id: ownership.DetectorPath, dets: []ownership.Detection{
		found(ownership.DetectorPath, "jq", catalog.ProvBrew, "/opt/homebrew/bin/jq", ""),
		found(ownership.DetectorPath, "gemini-cli", catalog.ProvNPM, "/opt/homebrew/bin/gemini", ""),
	}}
	brew := &fakeDet{id: ownership.DetectorBrew, dets: []ownership.Detection{found(ownership.DetectorBrew, "jq", catalog.ProvBrew, "", "brew-formula:jq")}}
	inv := &memInventory{records: []inventory.Record{{ID: "jq", State: inventory.StateManaged, Backend: catalog.BackendBrewFormula, BackendRef: "brew-formula:jq", InstalledAt: t0, InstalledBy: "j@t"}}}
	l := &lock{}
	res, err := newService(inv, l, path, brew).Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]ownership.State{"jq": ownership.StateManaged, "gh": ownership.StateAbsent, "iterm2": ownership.StateAbsent,
		"paperclip": ownership.StateUnavailableHere, "antigravity-cli": ownership.StateAbsent, "gemini-cli": ownership.StateForeign}
	if len(res.Statuses) != len(want) {
		t.Fatalf("statuses = %+v", res.Statuses)
	}
	for i, st := range res.Statuses {
		if st.PackageID != testCatalog().Entries[i].ID || st.State != want[st.PackageID] {
			t.Errorf("%d: %s = %s, want %s", i, st.PackageID, st.State, want[st.PackageID])
		}
	}
	if gem, _ := res.Status("gemini-cli"); gem.Offered {
		t.Error("gemini-cli is never offered")
	}
	if len(res.Findings) != 1 || res.Findings[0].Code != platform.CodeSuperseded || res.Findings[0].Severity != platform.SeverityInfo ||
		!strings.Contains(res.Findings[0].Message, "superseded by Google Antigravity CLI (agy)") {
		t.Errorf("findings = %+v", res.Findings)
	}
	if !res.Wrote || !inv.marked || !l.unlocked || len(inv.snaps) != 2 {
		t.Errorf("wrote=%v marked=%v unlocked=%v snaps=%+v", res.Wrote, inv.marked, l.unlocked, inv.snaps)
	}
	for _, s := range inv.snaps {
		if !s.ScannedAt.Equal(t0) || !s.Found {
			t.Errorf("snapshot %+v", s)
		}
	}
	// detectors see only the entries they cover.
	if !slices.Equal(brew.got, []string{"jq", "gh", "iterm2", "antigravity-cli"}) {
		t.Errorf("brew covered %v", brew.got)
	}
	if !slices.Equal(path.got, []string{"jq", "gh", "paperclip", "antigravity-cli", "gemini-cli"}) {
		t.Errorf("path covered %v", path.got)
	}
	if _, ok := res.Status("nope"); ok {
		t.Error("unknown status")
	}
	if m := res.StatusMap(); len(m) != 6 || m["jq"].State != ownership.StateManaged {
		t.Error("status map")
	}
}

func TestScanDetectorsRunConcurrently(t *testing.T) {
	var started sync.WaitGroup
	all := make(chan struct{})
	started.Add(2)
	a := &fakeDet{id: ownership.DetectorPath, started: &started, all: all}
	b := &fakeDet{id: ownership.DetectorBrew, started: &started, all: all}
	go func() { started.Wait(); close(all) }()
	done := make(chan error)
	go func() {
		_, err := newService(&memInventory{}, &lock{}, a, b).Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin}})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("detectors did not run concurrently")
	}
}

func TestScanTimeoutAndFailureAreUnknown(t *testing.T) {
	slow := &fakeDet{id: ownership.DetectorBundle, block: true}
	broken := &fakeDet{id: ownership.DetectorBrew, err: errors.New("brew info returned invalid JSON")}
	path := &fakeDet{id: ownership.DetectorPath}
	s := newService(&memInventory{}, &lock{}, slow, broken, path)
	s.Timeout = 50 * time.Millisecond
	res, err := s.Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin}})
	if err != nil {
		t.Fatal(err)
	}
	iterm, _ := res.Status("iterm2")
	if !iterm.Incomplete || iterm.State != ownership.StateAbsent {
		t.Errorf("iterm2 = %+v", iterm)
	}
	var reasons []string
	for _, d := range iterm.Detections {
		reasons = append(reasons, d.Reason)
	}
	if !slices.Contains(reasons, "bundle detector timed out after 50ms") || !slices.Contains(reasons, "brew detector: brew info returned invalid JSON") {
		t.Errorf("reasons = %v", reasons)
	}
	if pc, _ := res.Status("paperclip"); pc.Incomplete {
		t.Error("paperclip is not covered by brew or bundle")
	}
}

func TestScanCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newService(&memInventory{}, &lock{}, &fakeDet{id: ownership.DetectorPath, block: true}).Run(ctx, Input{Catalog: testCatalog(), Env: Env{Platform: darwin}})
	if err == nil {
		t.Error("cancelled scan must fail")
	}
}

func TestScanWithoutLockWritesNothing(t *testing.T) {
	inv := &memInventory{interrupted: []inventory.Operation{{ID: 7, PackageID: "gh", Op: inventory.OpInstall}}}
	res, err := newService(inv, &lock{holder: 4242}, &fakeDet{id: ownership.DetectorPath}).Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Wrote || inv.marked || len(inv.snaps) != 0 || len(inv.upserts) != 0 {
		t.Errorf("wrote without the lock: %+v %+v", res, inv)
	}
	if len(res.Findings) != 1 || res.Findings[0].Code != platform.CodeStateLocked || !strings.Contains(res.Findings[0].Message, "4242") {
		t.Errorf("findings = %+v", res.Findings)
	}
	// a caller holding the lock writes without TryLock.
	res, err = newService(inv, &lock{holder: 4242}).Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin}, HoldsLock: true})
	if err != nil || !res.Wrote || !inv.marked {
		t.Errorf("holds lock: %+v %v", res, err)
	}
}

// Review FR-005b: an unreadable lock holder (empty or garbage lock file,
// read as 0) is reported as "PID unknown", never "PID 0".
func TestScanLockHolderUnknown(t *testing.T) {
	res, err := newService(&memInventory{}, &lock{held: true}, &fakeDet{id: ownership.DetectorPath}).Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 || !strings.Contains(res.Findings[0].Message, "PID unknown") || strings.Contains(res.Findings[0].Message, "PID 0") {
		t.Errorf("findings = %+v", res.Findings)
	}
}

// Review FR-002: a relative or empty PATH entry gives one warning finding.
func TestScanWarnsAboutRelativePathEntries(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want string // "" means no finding
	}{
		{"absolute only", []string{"/opt/homebrew/bin", "/usr/bin"}, ""},
		{"dot", []string{".", "/usr/bin"}, `"."`},
		{"empty entry", []string{"/usr/bin", ""}, `""`},
		{"relative dir", []string{"node_modules/.bin", "/usr/bin"}, `"node_modules/.bin"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := newService(&memInventory{}, &lock{}).Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin, Path: tt.path}, ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			var got []platform.Finding
			for _, f := range res.Findings {
				if f.Code == platform.CodePathRelativeEntry {
					got = append(got, f)
				}
			}
			if tt.want == "" {
				if len(got) != 0 {
					t.Errorf("findings = %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Severity != platform.SeverityWarning || !strings.Contains(got[0].Message, tt.want) || !strings.Contains(got[0].Fix, "PATH") {
				t.Errorf("findings = %+v, want one warning naming %s", got, tt.want)
			}
		})
	}
}

func TestScanReadOnlyWritesNothing(t *testing.T) {
	inv := &memInventory{interrupted: []inventory.Operation{{ID: 7, PackageID: "gh", Op: inventory.OpInstall}}}
	l := &lock{}
	res, err := newService(inv, l, &fakeDet{id: ownership.DetectorPath}).Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin}, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Wrote || inv.marked || len(inv.snaps) != 0 || len(inv.upserts) != 0 || l.unlocked {
		t.Errorf("a read-only scan wrote or locked: %+v %+v", res, inv)
	}
	if len(res.Findings) != 0 || len(res.Statuses) != len(testCatalog().Entries) {
		t.Errorf("result = %+v", res)
	}
}

func TestScanReconcileInterrupted(t *testing.T) {
	cmd := func(argv ...string) []plan.Command { return []plan.Command{{Argv: argv, Mutating: true}} }
	ops := []inventory.Operation{
		{ID: 7, StartedAt: t0, PackageID: "gh", Op: inventory.OpInstall, Commands: cmd("brew", "install", "gh"), Status: inventory.StatusInterrupted, ConsentBy: "j@2026-11-14T19:00:00Z"},
		{ID: 8, StartedAt: t0, PackageID: "iterm2", Op: inventory.OpInstall, Commands: cmd("brew", "install", "--cask", "iterm2"), ConsentBy: "j@x"},
		{ID: 9, StartedAt: t0, PackageID: "homebrew", Op: inventory.OpBootstrap, Commands: cmd("/bin/bash", "/tmp/install.sh"), ConsentBy: "j@x"},
		{ID: 10, PackageID: "jq", Op: inventory.OpInstall, Commands: cmd("brew", "install", "jq")},
		{ID: 11, PackageID: "orca", Op: inventory.OpTap, Commands: cmd("brew", "tap", "stablyai/orca")},
		{ID: 12, PackageID: "x", Op: inventory.OpInstall, Commands: cmd("weird")},
	}
	brew := &fakeDet{id: ownership.DetectorBrew, dets: []ownership.Detection{
		{PackageID: "gh", Detector: ownership.DetectorBrew, Status: ownership.DetectionOK, Found: true, Provenance: catalog.ProvBrew, BackendRef: "brew-formula:gh", Version: "2.62.0"},
	}}
	tests := []struct {
		name     string
		brewPath string
		want     []string
	}{
		{"brew found", "/opt/homebrew/bin/brew", []string{"gh", "homebrew"}},
		{"brew still missing", "", []string{"gh"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := &memInventory{interrupted: ops, records: []inventory.Record{{ID: "jq", State: inventory.StateManaged, Backend: catalog.BackendBrewFormula, InstalledAt: t0, InstalledBy: "j"}}}
			res, err := newService(inv, &lock{}, brew).Run(context.Background(), Input{Catalog: testCatalog(), Env: Env{Platform: darwin, BrewPath: tt.brewPath}})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(res.Reconciled, tt.want) || len(res.Interrupted) != len(ops) {
				t.Errorf("reconciled = %v", res.Reconciled)
			}
			gh := res.Records["gh"]
			if gh.State != inventory.StateManaged || gh.Backend != catalog.BackendBrewFormula || gh.BackendRef != "brew-formula:gh" ||
				gh.InstalledBy != "j@2026-11-14T19:00:00Z" || gh.InstalledVersion != "2.62.0" || gh.ManifestRev != "embedded:test" {
				t.Errorf("gh record = %+v", gh)
			}
			if err := gh.Validate(); err != nil {
				t.Error(err)
			}
			if st, _ := res.Status("gh"); st.State != ownership.StateManaged {
				t.Errorf("gh = %s, want managed (AD-8)", st.State)
			}
			if st, _ := res.Status("iterm2"); st.State != ownership.StateAbsent {
				t.Errorf("iterm2 = %s, want absent so apply proposes it again", st.State)
			}
			if hb, ok := res.Records["homebrew"]; ok && (!hb.Bootstrap || hb.Backend != inventory.BackendScript) {
				t.Errorf("homebrew = %+v", hb)
			}
		})
	}
}

func TestScanErrors(t *testing.T) {
	in := Input{Catalog: testCatalog(), Env: Env{Platform: darwin}}
	if _, err := newService(&memInventory{}, &lock{err: errors.New("unwritable")}).Run(context.Background(), in); failure.ExitCode(err) != 4 {
		t.Errorf("lock error = %v", err)
	}
	if _, err := newService(&memInventory{err: errors.New("db gone")}, &lock{}).Run(context.Background(), in); failure.ExitCode(err) != 4 {
		t.Errorf("records error = %v", err)
	}
	for _, inv := range []Inventory{failingInv{mark: true}, failingInv{save: true}, failingInv{upsert: true}} {
		s := newService(nil, &lock{}, &fakeDet{id: ownership.DetectorBrew, dets: []ownership.Detection{found(ownership.DetectorBrew, "gh", catalog.ProvBrew, "", "brew-formula:gh")}})
		s.Inventory = inv
		if _, err := s.Run(context.Background(), in); failure.ExitCode(err) != 4 {
			t.Errorf("%+v: err = %v", inv, err)
		}
	}
}

type failingInv struct{ mark, save, upsert bool }

func (f failingInv) Records(context.Context) ([]inventory.Record, error) { return nil, nil }
func (f failingInv) UpsertRecord(context.Context, inventory.Record) error {
	if f.upsert {
		return errors.New("upsert")
	}
	return nil
}
func (f failingInv) MarkInterrupted(context.Context) ([]inventory.Operation, error) {
	if f.mark {
		return nil, errors.New("mark")
	}
	return []inventory.Operation{{PackageID: "gh", Op: inventory.OpInstall, Commands: []plan.Command{{Argv: []string{"brew", "install", "gh"}}}}}, nil
}
func (f failingInv) SaveDetections(context.Context, []inventory.DetectionSnapshot) error {
	if f.save {
		return errors.New("save")
	}
	return nil
}

func TestApplyConflictsAndCovers(t *testing.T) {
	res := Result{Statuses: []ownership.PackageStatus{{PackageID: "a", State: ownership.StateAbsent}, {PackageID: "b", State: ownership.StateForeign}}}
	c := &plan.Conflict{PackageID: "a", Path: "/x"}
	res.ApplyConflicts(map[string]*plan.Conflict{"a": c, "b": {PackageID: "b"}})
	if res.Statuses[0].State != ownership.StateConflict || res.Statuses[0].Conflict != c || res.Statuses[1].State != ownership.StateForeign {
		t.Errorf("statuses = %+v", res.Statuses)
	}
	e := catalog.Entry{Detect: catalog.DetectSpec{BundleIDs: []string{"x"}}}
	if Covers(ownership.DetectorBundle, e, platform.Platform{OS: "linux"}) || !Covers(ownership.DetectorBundle, e, darwin) || !Covers("other", e, darwin) {
		t.Error("covers")
	}
	s := &Service{}
	if len(s.snapshots([]ownership.PackageStatus{{PackageID: "x", Detections: []ownership.Detection{{}}}})) != 1 {
		t.Error("snapshot with default clock")
	}
}
