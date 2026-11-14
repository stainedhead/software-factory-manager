package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
)

// rowKind is what a Plan screen row shows.
type rowKind int

const (
	rowHeader rowKind = iota
	rowItem
	rowMember
)

// row is one line of the Plan screen table.
type row struct {
	kind rowKind
	// id is the package (item and member rows).
	id string
	// group is the choice group of a member row.
	group string
	// text is a header's title.
	text string
	// checked shows the tick: selected item or picked member.
	checked bool
	// selectable rows toggle with space.
	selectable bool
	// greyed rows can never be installed here.
	greyed bool
	// box is false for rows that show no checkbox (present packages).
	box bool
	// state is the ownership label.
	state string
	// note is the short reason or evidence.
	note string
	// why explains a row that cannot be toggled.
	why string
}

// section is one group of the Plan table, by ownership state.
type section struct {
	title string
	match func(st ownership.PackageStatus) bool
}

// sections are the Plan groups in display order: what can be installed
// first, then what blocks an install, then what is already there.
var sections = []section{
	{"absent: missing, can be installed", func(st ownership.PackageStatus) bool {
		return st.State == ownership.StateAbsent && !st.Incomplete
	}},
	{"conflict: a file is in the way (i for evidence)", func(st ownership.PackageStatus) bool { return st.State == ownership.StateConflict }},
	{"unavailable-here: not installable on this machine", func(st ownership.PackageStatus) bool {
		return st.State == ownership.StateUnavailableHere
	}},
	{"detection incomplete: never installed on top of the unknown", func(st ownership.PackageStatus) bool {
		return st.State == ownership.StateAbsent && st.Incomplete
	}},
	{"drifted: recorded, but not found as installed", func(st ownership.PackageStatus) bool { return st.State == ownership.StateDrifted }},
	{"duplicate: installed more than once", func(st ownership.PackageStatus) bool { return st.State == ownership.StateDuplicate }},
	{"foreign: installed by you, not by facman (never touched)", func(st ownership.PackageStatus) bool {
		return st.State == ownership.StateForeign
	}},
	{"managed: installed by facman", func(st ownership.PackageStatus) bool { return st.State == ownership.StateManaged }},
	{"adopted: handed over to facman", func(st ownership.PackageStatus) bool { return st.State == ownership.StateAdopted }},
}

// buildRows lays out the Plan table from what the session resolved and
// scanned: one row per wanted package grouped by ownership state, and the
// choice groups with their members as single-select rows.
func buildRows(cat catalog.Catalog, d catalog.DesiredSet, statuses map[string]ownership.PackageStatus) []row {
	var rows []row
	var groups []catalog.DesiredItem
	bySection := make([][]row, len(sections))
	for _, it := range d.Items {
		e, ok := cat.Entry(it.ID)
		if !ok {
			continue
		}
		if e.IsChoice() {
			groups = append(groups, it)
			continue
		}
		if !it.Selected && !it.Required && len(it.FromProfiles) == 0 {
			continue
		}
		st, ok := statuses[it.ID]
		if !ok || (!st.Offered && !st.Present()) {
			continue
		}
		for i, s := range sections {
			if s.match(st) {
				bySection[i] = append(bySection[i], itemRow(it, st))
				break
			}
		}
	}
	if len(groups) > 0 {
		rows = append(rows, row{kind: rowHeader, text: "choose one (or none)"})
		for _, g := range groups {
			e, _ := cat.Entry(g.ID)
			rows = append(rows, choiceRows(e, g, statuses)...)
		}
	}
	for i, s := range sections {
		if len(bySection[i]) == 0 {
			continue
		}
		rows = append(rows, row{kind: rowHeader, text: fmt.Sprintf("%s (%d)", s.title, len(bySection[i]))})
		rows = append(rows, bySection[i]...)
	}
	return rows
}

func itemRow(it catalog.DesiredItem, st ownership.PackageStatus) row {
	r := row{kind: rowItem, id: it.ID, state: stateLabel(st)}
	switch st.State {
	case ownership.StateAbsent:
		r.box, r.checked = true, it.Selected
		switch {
		case st.Incomplete:
			r.greyed, r.checked = true, false
			r.note = incompleteReason(st)
			r.why = it.ID + " is not offered: a detector could not answer, and facman never installs on top of the unknown"
		case len(it.FromProfiles) == 0:
			r.note = "needed by " + strings.Join(it.RequiredBy, ", ")
			r.why = it.ID + " is needed by " + strings.Join(it.RequiredBy, ", ") + "; untick that instead"
		default:
			r.selectable = true
			r.note = wantedNote(it)
		}
	case ownership.StateConflict:
		r.box, r.greyed, r.checked = true, true, false
		r.note = conflictNote(st)
		r.why = it.ID + " cannot be installed while something else holds its path (press i for the evidence)"
	case ownership.StateUnavailableHere:
		r.box, r.greyed, r.checked = true, true, false
		r.note = joinNonEmpty(st.UnavailableReason, st.Hint)
		r.why = it.ID + " is not installable here: " + r.note
	case ownership.StateForeign:
		r.note = presentNote(st) + "; adopt arrives in Increment 2"
		r.why = it.ID + " was installed by you, not facman; facman never touches it"
	case ownership.StateDuplicate:
		r.note = presentNote(st) + " wins on PATH"
		r.why = it.ID + " is installed more than once; resolve it yourself"
	default:
		r.note = presentNote(st)
		r.why = it.ID + " is already " + string(st.State)
	}
	return r
}

func choiceRows(e catalog.Entry, g catalog.DesiredItem, statuses map[string]ownership.PackageStatus) []row {
	rows := []row{{kind: rowItem, id: g.ID, state: "choice", note: choiceNote(e, g, statuses),
		why: "pick a member below with space (space on the picked one leaves the group empty)"}}
	satisfied := presentMember(e, statuses)
	for _, m := range e.Choice {
		st, ok := statuses[m.ID]
		if !ok || (!st.Offered && !st.Present()) {
			continue
		}
		r := row{kind: rowMember, id: m.ID, group: g.ID, state: stateLabel(st), checked: g.ChoicePick == m.ID, box: true}
		switch {
		case satisfied != "":
			r.greyed, r.why = true, g.ID+" is already satisfied by "+satisfied
			r.note = presentNote(st)
			if !st.Present() {
				r.note = ""
			}
		case st.State == ownership.StateAbsent && !st.Incomplete:
			r.selectable = true
		default:
			r.greyed = true
			r.note = joinNonEmpty(st.UnavailableReason, st.Hint, conflictNote(st))
			r.why = m.ID + " cannot be picked here: " + stateLabel(st)
		}
		rows = append(rows, r)
	}
	return rows
}

func choiceNote(e catalog.Entry, g catalog.DesiredItem, statuses map[string]ownership.PackageStatus) string {
	if p := presentMember(e, statuses); p != "" {
		return "satisfied by " + p
	}
	if g.ChoicePick != "" {
		return "picked " + g.ChoicePick
	}
	if g.Required {
		return "required: pick one"
	}
	return "optional: none picked"
}

func presentMember(e catalog.Entry, statuses map[string]ownership.PackageStatus) string {
	for _, m := range e.Choice {
		if st, ok := statuses[m.ID]; ok && st.Present() {
			return m.ID
		}
	}
	return ""
}

func wantedNote(it catalog.DesiredItem) string {
	from := strings.Join(it.FromProfiles, ", ")
	if it.Required {
		return "required by " + from
	}
	return "optional in " + from
}

// stateLabel shows OS-bundled foreign packages as "present (OS)" (FR-020).
func stateLabel(st ownership.PackageStatus) string {
	label := string(st.State)
	if st.State == ownership.StateForeign && st.OSBundled {
		label = "present (OS)"
	}
	return label
}

func presentNote(st ownership.PackageStatus) string {
	w := st.Winner
	if w == nil {
		return ""
	}
	return joinNonEmpty(string(w.Provenance), w.Version, w.Path)
}

func conflictNote(st ownership.PackageStatus) string {
	c := st.Conflict
	if c == nil {
		return ""
	}
	note := c.Path + " held by " + string(c.Owner)
	if c.Evidence.Dangling {
		note += " (dangling symlink)"
	}
	return note
}

func incompleteReason(st ownership.PackageStatus) string {
	var reasons []string
	for _, d := range st.Detections {
		if d.Status == ownership.DetectionUnknown && d.Reason != "" && !slices.Contains(reasons, d.Reason) {
			reasons = append(reasons, d.Reason)
		}
	}
	return "detection incomplete: " + strings.Join(reasons, "; ")
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "; ")
}

// filterRows keeps the rows whose package ID or display name contains f
// (any case), and the headers of the sections that keep any.
func filterRows(rows []row, cat catalog.Catalog, f string) []row {
	if f == "" {
		return rows
	}
	f = strings.ToLower(f)
	var out []row
	for _, r := range rows {
		if r.kind == rowHeader {
			out = append(out, r)
			continue
		}
		name := r.id
		if e, ok := cat.Entry(r.id); ok {
			name += " " + e.Display
		}
		if strings.Contains(strings.ToLower(name), f) {
			out = append(out, r)
		}
	}
	// drop headers with no rows after them
	var kept []row
	for i, r := range out {
		if r.kind == rowHeader && (i+1 == len(out) || out[i+1].kind == rowHeader) {
			continue
		}
		kept = append(kept, r)
	}
	return kept
}

// moveCursor moves from cur by delta to the next non-header row; a cur of
// -1 with delta +1 finds the first one.
func moveCursor(rows []row, cur, delta int) int {
	for i := cur + delta; i >= 0 && i < len(rows); i += delta {
		if rows[i].kind != rowHeader {
			return i
		}
	}
	if cur >= 0 && cur < len(rows) && rows[cur].kind != rowHeader {
		return cur
	}
	for i, r := range rows {
		if r.kind != rowHeader {
			return i
		}
	}
	return 0
}

func cursorRow(rows []row, cur int) (row, bool) {
	if cur < 0 || cur >= len(rows) || rows[cur].kind == rowHeader {
		return row{}, false
	}
	return rows[cur], true
}

// rows returns the Plan table for the current selection.
func (m *Model) rows() []row {
	res := m.res
	if res.Catalog == nil || res.Scan == nil {
		return nil
	}
	return buildRows(*res.Catalog, m.desired, res.Scan.StatusMap())
}

func (m *Model) visibleRows() []row {
	rows := m.rows()
	if m.res.Catalog == nil {
		return rows
	}
	return filterRows(rows, *m.res.Catalog, m.filter)
}

// toggle turns space on a row into a change of the selection, then lets
// the session resolve it again (requires are re-expanded there). A
// selection the session rejects is undone.
func (m *Model) toggle(r row) {
	if !r.selectable {
		m.status = r.why
		return
	}
	prev := cloneSelection(m.sel)
	if r.kind == rowMember {
		g, _ := m.desired.Item(r.group)
		m.sel.With = remove(m.sel.With, r.id)
		m.sel.Without = remove(m.sel.Without, r.id)
		if g.ChoicePick != r.id {
			m.sel.With = append(remove(m.sel.With, g.ChoicePick), r.id)
		}
	} else {
		it, _ := m.desired.Item(r.id)
		if it.Selected {
			m.sel.With = remove(m.sel.With, r.id)
			if it.Required {
				m.sel.Without = append(m.sel.Without, r.id)
			}
		} else {
			m.sel.Without = remove(m.sel.Without, r.id)
			if !it.Required {
				m.sel.With = append(m.sel.With, r.id)
			}
		}
	}
	d, err := m.steps.Resolve(m.sel)
	if err != nil {
		m.sel = prev
		m.status = errorText(err)
		return
	}
	m.desired = d
}

func remove(list []string, id string) []string {
	return slices.DeleteFunc(slices.Clone(list), func(s string) bool { return s == id })
}
