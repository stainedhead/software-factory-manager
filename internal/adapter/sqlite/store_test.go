package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
)

var t0 = time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func open(t *testing.T) (*Store, *clock) {
	t.Helper()
	c := &clock{t: t0}
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "state.db"), c.now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, c
}

func TestFreshDatabase(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	var tables []string
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		tables = append(tables, n)
	}
	_ = rows.Close()
	if want := []string{"detections", "meta", "operations", "packages", "services", "tap_trust"}; !slices.Equal(tables, want) {
		t.Errorf("tables = %v, want %v", tables, want)
	}
	if v, _ := s.Meta(ctx, "schema_version"); v != "1" {
		t.Errorf("schema_version = %q", v)
	}
	id, _ := s.Meta(ctx, "machine_id")
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Errorf("machine_id = %q", id)
	}
	if v, _ := s.Meta(ctx, "nope"); v != "" {
		t.Errorf("missing meta = %q", v)
	}
	if err := s.SetMeta(ctx, "last_manifest_rev", "embedded:1"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Meta(ctx, "last_manifest_rev"); v != "embedded:1" {
		t.Errorf("last_manifest_rev = %q", v)
	}
	if s.Path() == "" {
		t.Error("Path empty")
	}
}

func TestReopenIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	ctx := context.Background()
	s, err := Open(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := s.Meta(ctx, "machine_id")
	_ = s.Close()
	s, err = Open(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if again, _ := s.Meta(ctx, "machine_id"); again != id {
		t.Errorf("machine_id changed on re-open: %s -> %s", id, again)
	}
}

func TestNewerSchemaRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	ctx := context.Background()
	s, err := Open(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta(ctx, "schema_version", "99"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, err = Open(ctx, path, nil)
	if failure.ExitCode(err) != 4 || !strings.Contains(err.Error(), "written by a newer facman") || !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v", err)
	}
	// a garbage schema_version is also a bootstrap error
	s2, _ := sql.Open("sqlite", "file:"+path)
	_, _ = s2.Exec(`UPDATE meta SET value = 'x' WHERE key = 'schema_version'`)
	_ = s2.Close()
	if _, err := Open(ctx, path, nil); failure.ExitCode(err) != 4 {
		t.Errorf("bad version err = %v", err)
	}
}

func TestGarbageFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	garbage := []byte("this is not a database, it is a shopping list\n")
	if err := os.WriteFile(path, garbage, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(context.Background(), path, nil)
	if failure.ExitCode(err) != 4 || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(garbage) {
		t.Error("garbage file was modified")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("extra files created next to the garbage DB: %v", entries)
	}
	short := filepath.Join(t.TempDir(), "short.db")
	_ = os.WriteFile(short, []byte("SQ"), 0o600)
	if _, err := Open(context.Background(), short, nil); failure.ExitCode(err) != 4 {
		t.Errorf("short file err = %v", err)
	}
}

func TestEmptyFileInitialised(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	_ = os.WriteFile(path, nil, 0o600)
	s, err := Open(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}

func TestOpenUnreadable(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(context.Background(), dir, nil); failure.ExitCode(err) != 4 {
		t.Errorf("directory as DB: %v", err)
	}
	if _, err := Open(context.Background(), filepath.Join(dir, "missing", "state.db"), nil); failure.ExitCode(err) != 4 {
		t.Errorf("missing parent: %v", err)
	}
}

func record(id string) inventory.Record {
	return inventory.Record{ID: id, State: inventory.StateManaged, Backend: catalog.BackendBrewFormula, BackendRef: "brew-formula:" + id,
		InstalledVersion: "1.0", InstallPath: "/opt/homebrew/bin/" + id, InstalledAt: t0, InstalledBy: "j@2026-11-14T19:00:00Z",
		ManifestRev: "embedded:dev"}
}

func TestRecords(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if err := s.UpsertRecord(ctx, record("jq")); err != nil {
		t.Fatal(err)
	}
	adopted := record("gh")
	at := t0.Add(time.Hour)
	adopted.State, adopted.AdoptedAt = inventory.StateAdopted, &at
	if err := s.UpsertRecord(ctx, adopted); err != nil {
		t.Fatal(err)
	}
	brew := inventory.Record{ID: "homebrew", State: inventory.StateManaged, Backend: inventory.BackendScript, InstalledAt: t0, InstalledBy: "j@x", Bootstrap: true}
	if err := s.UpsertRecord(ctx, brew); err != nil {
		t.Fatal(err)
	}
	updated := record("jq")
	updated.InstalledVersion = "1.7.1"
	if err := s.UpsertRecord(ctx, updated); err != nil {
		t.Fatal(err)
	}
	got, err := s.Records(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "gh" || got[1].ID != "homebrew" || got[2].ID != "jq" {
		t.Fatalf("records = %+v", got)
	}
	if !got[0].AdoptedAt.Equal(at) || got[0].State != inventory.StateAdopted {
		t.Errorf("gh = %+v", got[0])
	}
	if !got[1].Bootstrap || got[1].Backend != "script" || got[1].Scope != "user" || got[1].InstallPath != "" {
		t.Errorf("homebrew = %+v", got[1])
	}
	if got[2].InstalledVersion != "1.7.1" || !got[2].InstalledAt.Equal(t0) || got[2].ManifestRev != "embedded:dev" {
		t.Errorf("jq = %+v", got[2])
	}
	bad := record("x")
	bad.State = "foreign"
	if err := s.UpsertRecord(ctx, bad); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("foreign record err = %v", err)
	}
}

func TestOperationsRoundTrip(t *testing.T) {
	s, c := open(t)
	ctx := context.Background()
	cmds := []plan.Command{{Argv: []string{"brew", "install", "--cask", "claude-code"}, EnvNames: []string{"PATH"}, NeedsSudo: true, Interactive: true, Mutating: true}}
	id, err := s.StartOperation(ctx, inventory.Operation{StartedAt: t0, PackageID: "claude-code", Op: inventory.OpInstall,
		Commands: cmds, LogPath: "/logs/a.log", ConsentBy: "j@2026-11-14T19:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	ops, _ := s.Operations(ctx)
	if len(ops) != 1 || ops[0].Status != inventory.StatusInProgress || ops[0].FinishedAt != nil || ops[0].ExitCode != nil {
		t.Fatalf("started op = %+v", ops)
	}
	if !slices.EqualFunc(ops[0].Commands, cmds, func(a, b plan.Command) bool {
		return slices.Equal(a.Argv, b.Argv) && slices.Equal(a.EnvNames, b.EnvNames) && a.NeedsSudo == b.NeedsSudo && a.Interactive == b.Interactive && a.Mutating == b.Mutating
	}) {
		t.Errorf("commands = %+v", ops[0].Commands)
	}
	c.t = t0.Add(2 * time.Minute)
	if err := s.FinishOperation(ctx, id, 0, inventory.StatusSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	ops, _ = s.Operations(ctx)
	if ops[0].Status != inventory.StatusSucceeded || *ops[0].ExitCode != 0 || !ops[0].FinishedAt.Equal(c.t) || ops[0].LogPath != "/logs/a.log" {
		t.Errorf("finished op = %+v", ops[0])
	}
	if err := s.FinishOperation(ctx, id, 1, inventory.StatusFailed, ""); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("finishing twice: %v", err)
	}
	if err := s.FinishOperation(ctx, 999, 0, inventory.StatusSucceeded, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
	id2, _ := s.StartOperation(ctx, inventory.Operation{StartedAt: t0, PackageID: "gh", Op: inventory.OpInstall, ConsentBy: "j@x"})
	if err := s.FinishOperation(ctx, id2, 1, inventory.StatusFailed, "/logs/b.log"); err != nil {
		t.Fatal(err)
	}
	ops, _ = s.Operations(ctx)
	if ops[1].LogPath != "/logs/b.log" || *ops[1].ExitCode != 1 || len(ops[1].Commands) != 0 {
		t.Errorf("second op = %+v", ops[1])
	}
	for _, bad := range []inventory.Operation{
		{StartedAt: t0, PackageID: "gh", Op: inventory.OpInstall, ConsentBy: "j@x", Status: inventory.StatusSucceeded},
		{StartedAt: t0, PackageID: "gh", Op: inventory.OpInstall},
	} {
		if _, err := s.StartOperation(ctx, bad); !errors.Is(err, inventory.ErrInvalid) {
			t.Errorf("StartOperation(%+v) err = %v", bad, err)
		}
	}
}

func TestMarkInterrupted(t *testing.T) {
	s, c := open(t)
	ctx := context.Background()
	start := func(pkg string) int64 {
		id, err := s.StartOperation(ctx, inventory.Operation{StartedAt: t0, PackageID: pkg, Op: inventory.OpInstall, ConsentBy: "j@x"})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	done := start("jq")
	_ = s.FinishOperation(ctx, done, 0, inventory.StatusSucceeded, "")
	gh := start("gh")
	tmux := start("tmux")
	c.t = t0.Add(time.Hour)
	got, err := s.MarkInterrupted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != gh || got[1].ID != tmux || got[0].Status != inventory.StatusInterrupted || !got[0].FinishedAt.Equal(c.t) {
		t.Fatalf("interrupted = %+v", got)
	}
	ops, _ := s.Operations(ctx)
	if ops[0].Status != inventory.StatusSucceeded || ops[1].Status != inventory.StatusInterrupted || ops[1].ExitCode != nil {
		t.Errorf("ops = %+v", ops)
	}
	again, _ := s.MarkInterrupted(ctx)
	if len(again) != 0 {
		t.Errorf("second MarkInterrupted = %+v", again)
	}
	if err := s.FinishOperation(ctx, gh, 0, inventory.StatusSucceeded, ""); !errors.Is(err, inventory.ErrInvalid) {
		t.Error("an interrupted row must stay interrupted")
	}
}

func TestSaveDetectionsKeepsLastTenScans(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if err := s.SaveDetections(ctx, nil); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		at := t0.Add(time.Duration(i) * time.Minute)
		snaps := []inventory.DetectionSnapshot{
			{ScannedAt: at, PackageID: "jq", Found: true, Version: "1.7.1", Provenance: catalog.ProvBrew, Path: "/opt/homebrew/bin/jq"},
			{ScannedAt: at, PackageID: "gh"},
		}
		if err := s.SaveDetections(ctx, snaps); err != nil {
			t.Fatal(err)
		}
	}
	jq, _ := s.Detections(ctx, "jq")
	if len(jq) != 10 {
		t.Fatalf("jq rows = %d, want 10", len(jq))
	}
	if !jq[0].ScannedAt.Equal(t0.Add(11*time.Minute)) || !jq[9].ScannedAt.Equal(t0.Add(2*time.Minute)) {
		t.Errorf("kept scans %s .. %s", jq[0].ScannedAt, jq[9].ScannedAt)
	}
	if !jq[0].Found || jq[0].Provenance != catalog.ProvBrew || jq[0].Version != "1.7.1" || jq[0].Path == "" {
		t.Errorf("jq row = %+v", jq[0])
	}
	gh, _ := s.Detections(ctx, "gh")
	if len(gh) != 10 || gh[0].Found || gh[0].Provenance != "" {
		t.Errorf("gh rows = %+v", gh)
	}
}

func TestTapTrust(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	id, _ := s.StartOperation(ctx, inventory.Operation{StartedAt: t0, PackageID: "orca", Op: inventory.OpTrust, ConsentBy: "j@x"})
	if err := s.RecordTapTrust(ctx, inventory.TapTrustGrant{PackageID: "orca", Tap: "stablyai/orca", Item: "stablyai/orca/orca", GrantedAt: t0, OperationID: id}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordTapTrust(ctx, inventory.TapTrustGrant{PackageID: "x", Tap: "a/b", Item: "a/b/x", GrantedAt: t0}); err != nil {
		t.Fatal(err)
	}
	got, err := s.TapTrustGrants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].OperationID != id || got[0].Item != "stablyai/orca/orca" || !got[0].GrantedAt.Equal(t0) || got[1].OperationID != 0 {
		t.Errorf("grants = %+v", got)
	}
	if err := s.RecordTapTrust(ctx, inventory.TapTrustGrant{PackageID: "y", Tap: "a/b", Item: "a/b/y", GrantedAt: t0, OperationID: 999}); err == nil {
		t.Error("a grant for an unknown operation should violate the foreign key")
	}
}

func TestTimesAreRFC3339UTC(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	local := time.Date(2026, 11, 14, 14, 0, 0, 0, time.FixedZone("EST", -5*3600))
	r := record("jq")
	r.InstalledAt = local
	_ = s.UpsertRecord(ctx, r)
	var raw string
	_ = s.db.QueryRowContext(ctx, `SELECT installed_at FROM packages WHERE id = 'jq'`).Scan(&raw)
	if raw != "2026-11-14T19:00:00Z" {
		t.Errorf("installed_at stored as %q", raw)
	}
}

func TestReadableBySqlite3CLI(t *testing.T) {
	bin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 CLI not installed")
	}
	s, _ := open(t)
	_ = s.UpsertRecord(context.Background(), record("jq"))
	out, err := exec.Command(bin, s.Path(), "SELECT id, state FROM packages").Output()
	if err != nil || strings.TrimSpace(string(out)) != "jq|managed" {
		t.Errorf("sqlite3 output %q, %v", out, err)
	}
}

func TestClosedStoreErrors(t *testing.T) {
	s, _ := open(t)
	_ = s.Close()
	ctx := context.Background()
	checks := map[string]error{}
	_, checks["Records"] = s.Records(ctx)
	checks["UpsertRecord"] = s.UpsertRecord(ctx, record("jq"))
	_, checks["StartOperation"] = s.StartOperation(ctx, inventory.Operation{StartedAt: t0, PackageID: "x", Op: inventory.OpInstall, ConsentBy: "j"})
	checks["FinishOperation"] = s.FinishOperation(ctx, 1, 0, inventory.StatusSucceeded, "")
	_, checks["Operations"] = s.Operations(ctx)
	_, checks["MarkInterrupted"] = s.MarkInterrupted(ctx)
	checks["SaveDetections"] = s.SaveDetections(ctx, []inventory.DetectionSnapshot{{PackageID: "x"}})
	_, checks["Detections"] = s.Detections(ctx, "x")
	checks["RecordTapTrust"] = s.RecordTapTrust(ctx, inventory.TapTrustGrant{})
	_, checks["TapTrustGrants"] = s.TapTrustGrants(ctx)
	_, checks["Meta"] = s.Meta(ctx, "x")
	checks["SetMeta"] = s.SetMeta(ctx, "x", "y")
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s on a closed store returned nil", name)
		}
	}
}
