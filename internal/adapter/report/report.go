// Package report encodes facman's --json output as report schema v1
// (FR-007, data-dictionary §7). The schema is embedded (schema/report.v1.json)
// so tests and, later, export validate against the exact file shipped.
package report

import (
	_ "embed" // the schema file
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/appinfo"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/doctor"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/status"
	"github.com/stainedhead/software-factory-manager/internal/usecase/summary"
)

// Schema is report schema v1 (JSON Schema 2020-12).
//
//go:embed schema/report.v1.json
var Schema []byte

// SchemaID is the value of every document's schema field.
const SchemaID = "facman.report/v1"

// Input is everything one command produced; nil parts are left out.
type Input struct {
	// Command is scan, plan, apply, status or doctor.
	Command platform.Command
	// Version is the facman version.
	Version string
	// Now is the generation time.
	Now time.Time
	// Bootstrap is the bootstrap report, when the check ran.
	Bootstrap *platform.BootstrapReport
	// Catalog is the loaded catalog.
	Catalog *catalog.Catalog
	// Desired is the resolved profile selection (plan, apply).
	Desired *catalog.DesiredSet
	// Statuses are the scan results (scan, plan, apply).
	Statuses []ownership.PackageStatus
	// Findings are the scan findings (superseded tools, lock held).
	Findings []platform.Finding
	// Plan is the plan (plan, apply: the plan as run).
	Plan *plan.Plan
	// Outcome is the apply outcome (nil when nothing ran or was asked).
	Outcome *execute.Outcome
	// Status is the status view.
	Status *status.Report
	// Doctor is the doctor report.
	Doctor *doctor.Report
	// Summary is the apply summary.
	Summary *summary.Summary
	// Err is the error the command ends with.
	Err error
}

// Document is one report. Field names and shapes follow the schema.
type Document struct {
	Schema      string      `json:"schema"`
	Command     string      `json:"command"`
	Facman      Facman      `json:"facman"`
	GeneratedAt string      `json:"generated_at"`
	Platform    *Platform   `json:"platform,omitempty"`
	Bootstrap   *Bootstrap  `json:"bootstrap,omitempty"`
	Catalog     *Catalog    `json:"catalog,omitempty"`
	Packages    []Package   `json:"packages,omitempty"`
	Plan        *Plan       `json:"plan,omitempty"`
	Results     []Result    `json:"results,omitempty"`
	ConsentBy   *Nullable   `json:"consent_by,omitempty"`
	Findings    []Finding   `json:"findings,omitempty"`
	NextSteps   []NextStep  `json:"next_steps,omitempty"`
	TapTrust    []TapTrust  `json:"tap_trust,omitempty"`
	Error       *ErrorField `json:"error,omitempty"`
	ExitCode    int         `json:"exit_code"`
}

// Facman names the program.
type Facman struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

// Platform is the machine.
type Platform struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	WSL     bool   `json:"wsl"`
	Display bool   `json:"display"`
	Glibc   string `json:"glibc,omitempty"`
	Distro  string `json:"distro,omitempty"`
}

// Bootstrap is the bootstrap check.
type Bootstrap struct {
	Homebrew Homebrew  `json:"homebrew"`
	CLT      *bool     `json:"clt,omitempty"`
	Node     Node      `json:"node"`
	Findings []Finding `json:"findings"`
}

// Homebrew is where brew was found.
type Homebrew struct {
	Found              bool   `json:"found"`
	Path               string `json:"path,omitempty"`
	Prefix             string `json:"prefix,omitempty"`
	OnPath             bool   `json:"on_path"`
	Version            string `json:"version,omitempty"`
	PrefixOwnedByOther bool   `json:"prefix_owned_by_other,omitempty"`
}

// Node is the Node.js runtime.
type Node struct {
	Found      bool   `json:"found"`
	Version    string `json:"version,omitempty"`
	Provenance string `json:"provenance,omitempty"`
	Satisfies  bool   `json:"satisfies"`
	NPM        bool   `json:"npm,omitempty"`
}

// Finding is a bootstrap, scan or doctor finding.
type Finding struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	PackageID   string `json:"package_id,omitempty"`
	Message     string `json:"message"`
	Fix         string `json:"fix,omitempty"`
	BlocksApply bool   `json:"blocks_apply,omitempty"`
}

// Catalog identifies the catalog and profiles.
type Catalog struct {
	Revision string   `json:"revision"`
	Profiles []string `json:"profiles,omitempty"`
}

// Package is one catalog package (or inventory record, for status).
type Package struct {
	ID                string      `json:"id"`
	Display           string      `json:"display,omitempty"`
	State             string      `json:"state"`
	Offered           *bool       `json:"offered,omitempty"`
	OSBundled         bool        `json:"os_bundled,omitempty"`
	Satisfying        bool        `json:"satisfying,omitempty"`
	Provenance        string      `json:"provenance,omitempty"`
	Path              string      `json:"path,omitempty"`
	Version           string      `json:"version,omitempty"`
	Detections        []Detection `json:"detections,omitempty"`
	Winner            *Detection  `json:"winner,omitempty"`
	UnavailableReason string      `json:"unavailable_reason,omitempty"`
	Hint              string      `json:"hint,omitempty"`
	Conflict          *Conflict   `json:"conflict,omitempty"`
	Incomplete        bool        `json:"incomplete,omitempty"`
	Selected          *bool       `json:"selected,omitempty"`
	Required          *bool       `json:"required,omitempty"`
	Detail            string      `json:"detail,omitempty"`
	Record            *Record     `json:"record,omitempty"`
}

// Detection is one detector's answer.
type Detection struct {
	Detector     string `json:"detector"`
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
	Found        bool   `json:"found"`
	Path         string `json:"path,omitempty"`
	ResolvedPath string `json:"resolved_path,omitempty"`
	Provenance   string `json:"provenance,omitempty"`
	Version      string `json:"version,omitempty"`
	BackendRef   string `json:"backend_ref,omitempty"`
	PathWinner   bool   `json:"path_winner,omitempty"`
}

// Conflict is a conflicting path with its evidence (FR-016).
type Conflict struct {
	Path           string   `json:"path"`
	Owner          string   `json:"owner"`
	OwnerDetail    string   `json:"owner_detail,omitempty"`
	Evidence       Evidence `json:"evidence"`
	FailingCommand []string `json:"failing_command"`
	FixHint        string   `json:"fix_hint"`
}

// Evidence is what sits at a conflicting path.
type Evidence struct {
	LsLine        string `json:"ls_line"`
	FileType      string `json:"file_type,omitempty"`
	Symlink       bool   `json:"symlink"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
	Dangling      bool   `json:"dangling"`
	Executable    bool   `json:"executable"`
	Size          int64  `json:"size"`
	ModTime       string `json:"mod_time"`
}

// Record is an inventory row.
type Record struct {
	State            string `json:"state"`
	Backend          string `json:"backend"`
	BackendRef       string `json:"backend_ref"`
	InstalledVersion string `json:"installed_version,omitempty"`
	InstalledAt      string `json:"installed_at,omitempty"`
	InstalledBy      string `json:"installed_by,omitempty"`
	Bootstrap        bool   `json:"bootstrap,omitempty"`
}

// Plan is the ordered plan.
type Plan struct {
	Actions  []Action    `json:"actions"`
	Excluded []Exclusion `json:"excluded"`
}

// Action is one plan step.
type Action struct {
	ID        int       `json:"id"`
	Kind      string    `json:"kind"`
	PackageID string    `json:"package_id"`
	Commands  []Command `json:"commands"`
	DependsOn []int     `json:"depends_on"`
	Tap       string    `json:"tap,omitempty"`
	Item      string    `json:"item,omitempty"`
}

// Command is one exact argv.
type Command struct {
	Argv       []string `json:"argv"`
	EnvNames   []string `json:"env_names,omitempty"`
	NeedsSudo  bool     `json:"needs_sudo"`
	Unverified bool     `json:"unverified"`
}

// Exclusion is a wanted package left out.
type Exclusion struct {
	PackageID string `json:"package_id"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail,omitempty"`
}

// Result is how one action ended.
type Result struct {
	ActionID    int    `json:"action_id"`
	PackageID   string `json:"package_id"`
	Status      string `json:"status"`
	ExitCode    int    `json:"exit_code"`
	LogPath     string `json:"log_path,omitempty"`
	OperationID int64  `json:"operation_id,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// NextStep is a sign-in or shell-profile step after apply.
type NextStep struct {
	PackageID string `json:"package_id"`
	Message   string `json:"message"`
}

// TapTrust is an item-trust grant facman made (status).
type TapTrust struct {
	PackageID   string `json:"package_id"`
	Tap         string `json:"tap"`
	Item        string `json:"item"`
	GrantedAt   string `json:"granted_at"`
	OperationID int64  `json:"operation_id,omitempty"`
}

// ErrorField is the error a command ended with.
type ErrorField struct {
	Kind    string   `json:"kind"`
	Message string   `json:"message"`
	Fix     string   `json:"fix,omitempty"`
	Details []string `json:"details,omitempty"`
}

// Nullable is a string that encodes as null when Value is nil.
type Nullable struct {
	// Value is the string, or nil for null.
	Value *string
}

// MarshalJSON encodes the value or null.
func (n Nullable) MarshalJSON() ([]byte, error) {
	if n.Value == nil {
		return []byte("null"), nil
	}
	b, err := json.Marshal(*n.Value)
	if err != nil {
		return nil, fmt.Errorf("encoding consent: %w", err)
	}
	return b, nil
}

// Build turns the command's results into a document.
func Build(in Input) Document {
	d := Document{
		Schema:      SchemaID,
		Command:     string(in.Command),
		Facman:      Facman{Name: appinfo.Name, Title: appinfo.Title, Version: orDefault(in.Version, "dev")},
		GeneratedAt: stamp(in.Now),
		ExitCode:    failure.ExitCode(in.Err),
	}
	if in.Bootstrap != nil {
		d.Platform, d.Bootstrap = platformOf(in.Bootstrap.Platform), bootstrapOf(*in.Bootstrap)
	}
	if in.Catalog != nil {
		d.Catalog = &Catalog{Revision: orDefault(in.Catalog.Revision, "unknown")}
		if in.Desired != nil {
			d.Catalog.Profiles = in.Desired.Profiles
		}
	}
	if in.Status != nil {
		d.Packages, d.TapTrust = statusPackages(in), tapTrust(in.Status.TapTrust)
	} else if in.Command != platform.CommandDoctor {
		d.Packages = packages(in)
	}
	if in.Plan != nil {
		d.Plan = planOf(*in.Plan)
	}
	if in.Command == platform.CommandApply {
		d.ConsentBy = &Nullable{}
		if o := in.Outcome; o != nil {
			if !o.Consent.At.IsZero() {
				c := o.Consent.String()
				d.ConsentBy.Value = &c
			}
			d.Results = results(o.Results)
		}
		if in.Summary != nil {
			d.NextSteps = nextSteps(*in.Summary)
		}
	}
	d.Findings = findings(in)
	d.Error = errorOf(in.Err)
	return d
}

// Encode writes d as indented JSON with a trailing newline.
func Encode(w io.Writer, d Document) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return fmt.Errorf("writing the JSON report: %w", err)
	}
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func stamp(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format(time.RFC3339)
}

func platformOf(p platform.Platform) *Platform {
	out := &Platform{OS: orDefault(p.OS, "linux"), Arch: orDefault(p.Arch, "unknown"), WSL: p.WSL, Display: p.Display, Distro: p.Distro.ID}
	if p.Glibc != nil {
		out.Glibc = p.Glibc.Raw
	}
	return out
}

func bootstrapOf(r platform.BootstrapReport) *Bootstrap {
	h := r.Homebrew
	return &Bootstrap{
		Homebrew: Homebrew{Found: h.Found, Path: h.Path, Prefix: h.Prefix, OnPath: h.OnPATH, Version: h.Version, PrefixOwnedByOther: h.PrefixOwnedByOther},
		CLT:      r.CLT,
		Node:     Node{Found: r.Node.Found, Version: r.Node.Version, Provenance: r.Node.Provenance, Satisfies: r.Node.Satisfies, NPM: r.Node.NPM},
		Findings: findingList(r.Findings, nil),
	}
}

func findingList(fs []platform.Finding, ids func(string) string) []Finding {
	out := make([]Finding, 0, len(fs))
	for _, f := range fs {
		pkg := ""
		if ids != nil {
			pkg = ids(f.Message)
		}
		out = append(out, Finding{Code: f.Code, Severity: string(f.Severity), PackageID: pkg, Message: f.Message, Fix: f.Fix, BlocksApply: f.BlocksApply})
	}
	return out
}

// findings: doctor findings with their package (doctor messages start
// with "<package id>: "), or the scan findings for the other commands.
func findings(in Input) []Finding {
	if in.Doctor != nil {
		return findingList(in.Doctor.Findings, func(msg string) string {
			id, _, ok := strings.Cut(msg, ": ")
			if ok && in.Catalog != nil && in.Catalog.Index(id) >= 0 {
				return id
			}
			return ""
		})
	}
	if len(in.Findings) == 0 {
		return nil
	}
	return findingList(in.Findings, nil)
}

func packages(in Input) []Package {
	out := make([]Package, 0, len(in.Statuses))
	for _, st := range in.Statuses {
		p := Package{ID: st.PackageID, State: string(st.State), OSBundled: st.OSBundled, Satisfying: st.Satisfying,
			UnavailableReason: st.UnavailableReason, Hint: st.Hint, Incomplete: st.Incomplete, Conflict: conflictOf(st.Conflict),
			Record: recordOf(st.Record)}
		offered := st.Offered
		p.Offered = &offered
		if in.Catalog != nil {
			if e, ok := in.Catalog.Entry(st.PackageID); ok {
				p.Display = e.Display
			}
		}
		for _, det := range st.Detections {
			p.Detections = append(p.Detections, detectionOf(det))
		}
		if w := st.Winner; w != nil {
			wd := detectionOf(*w)
			p.Winner, p.Provenance, p.Path, p.Version = &wd, string(w.Provenance), w.Path, w.Version
		}
		if in.Desired != nil {
			it, _ := in.Desired.Item(st.PackageID)
			sel, req := it.Selected, it.Required
			p.Selected, p.Required = &sel, &req
		}
		out = append(out, p)
	}
	return out
}

func statusPackages(in Input) []Package {
	r := in.Status
	var out []Package
	for _, row := range r.Rows {
		out = append(out, Package{ID: row.PackageID, Display: row.Display, State: string(row.State), Provenance: string(row.Provenance),
			Path: row.Path, Version: row.Version, Detail: row.Detail, Record: recordOf(row.Record)})
	}
	if r.Homebrew != nil {
		out = append(out, Package{ID: r.Homebrew.ID, Display: "Homebrew", State: string(r.Homebrew.State), Record: recordOf(r.Homebrew),
			Detail: "installed by facman (bootstrap)"})
	}
	return out
}

func detectionOf(d ownership.Detection) Detection {
	return Detection{Detector: string(d.Detector), Status: string(d.Status), Reason: d.Reason, Found: d.Found, Path: d.Path,
		ResolvedPath: d.ResolvedPath, Provenance: string(d.Provenance), Version: d.Version, BackendRef: d.BackendRef, PathWinner: d.PathWinner}
}

func conflictOf(c *plan.Conflict) *Conflict {
	if c == nil {
		return nil
	}
	ev := c.Evidence
	failing := c.FailingCommand
	if len(failing) == 0 {
		failing = []string{"unknown"}
	}
	return &Conflict{Path: c.Path, Owner: string(c.Owner), OwnerDetail: c.OwnerDetail, FailingCommand: failing, FixHint: orDefault(c.FixHint, "remove it yourself"),
		Evidence: Evidence{LsLine: ev.LsLine, FileType: ev.FileType, Symlink: ev.IsSymlink, SymlinkTarget: ev.SymlinkTarget, Dangling: ev.Dangling,
			Executable: ev.Executable, Size: max(ev.Size, 0), ModTime: stamp(ev.ModTime)}}
}

func recordOf(r *inventory.Record) *Record {
	if r == nil {
		return nil
	}
	out := &Record{State: string(r.State), Backend: string(r.Backend), BackendRef: r.BackendRef, InstalledVersion: r.InstalledVersion,
		InstalledBy: r.InstalledBy, Bootstrap: r.Bootstrap}
	if !r.InstalledAt.IsZero() {
		out.InstalledAt = stamp(r.InstalledAt)
	}
	return out
}

func planOf(p plan.Plan) *Plan {
	out := &Plan{Actions: []Action{}, Excluded: []Exclusion{}}
	for _, a := range p.Actions {
		act := Action{ID: a.ID, Kind: string(a.Kind), PackageID: a.PackageID, DependsOn: append([]int{}, a.DependsOn...)}
		if a.TapTrust != nil {
			act.Tap, act.Item = a.TapTrust.Tap, a.TapTrust.Item
		}
		for _, c := range a.Commands {
			act.Commands = append(act.Commands, Command{Argv: c.Argv, EnvNames: c.EnvNames, NeedsSudo: c.NeedsSudo, Unverified: c.Unverified})
		}
		out.Actions = append(out.Actions, act)
	}
	for _, ex := range p.Excluded {
		out.Excluded = append(out.Excluded, Exclusion{PackageID: ex.PackageID, Reason: string(ex.Reason), Detail: ex.Detail})
	}
	return out
}

func results(rs []execute.ActionResult) []Result {
	out := make([]Result, 0, len(rs))
	for _, r := range rs {
		out = append(out, Result{ActionID: r.ActionID, PackageID: r.PackageID, Status: string(r.Status), ExitCode: r.ExitCode,
			LogPath: r.LogPath, OperationID: r.OperationID, Detail: r.Detail})
	}
	return out
}

// nextSteps pairs each sign-in step with its package (summary lists them
// in line order) and adds the shell profile lines.
func nextSteps(s summary.Summary) []NextStep {
	var out []NextStep
	i := 0
	for _, l := range s.Lines {
		if l.Installed && l.Text == "installed, sign-in required" && i < len(s.NextSteps) {
			out = append(out, NextStep{PackageID: l.PackageID, Message: s.NextSteps[i]})
			i++
		}
	}
	for _, line := range s.ProfileLines {
		pkg := "node"
		if strings.Contains(line, "shellenv") {
			pkg = plan.HomebrewPackageID
		}
		out = append(out, NextStep{PackageID: pkg, Message: "add to your shell profile: " + line})
	}
	return out
}

func tapTrust(gs []inventory.TapTrustGrant) []TapTrust {
	var out []TapTrust
	for _, g := range gs {
		out = append(out, TapTrust{PackageID: g.PackageID, Tap: g.Tap, Item: g.Item, GrantedAt: stamp(g.GrantedAt), OperationID: g.OperationID})
	}
	return out
}

func errorOf(err error) *ErrorField {
	if err == nil {
		return nil
	}
	var fe *failure.Error
	if !errors.As(err, &fe) {
		return &ErrorField{Kind: string(failure.KindOperation), Message: err.Error()}
	}
	return &ErrorField{Kind: string(fe.Kind), Message: orDefault(fe.Msg, err.Error()), Fix: fe.Fix, Details: fe.Details}
}
