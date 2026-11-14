package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/stainedhead/software-factory-manager/internal/domain/appinfo"
	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/ownership"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

var (
	boldStyle  = lipgloss.NewStyle().Bold(true)
	faintStyle = lipgloss.NewStyle().Faint(true)
	errStyle   = lipgloss.NewStyle().Bold(true)
)

// spinnerFrames are ASCII so golden files stay stable.
var spinnerFrames = []string{"|", "/", "-", `\`}

// errNoHandoff is returned when no hand-off factory is wired.
var errNoHandoff = errors.New("no terminal hand-off is available for an interactive command")

// chromeLines is the height of the header, rules and footer.
const chromeLines = 7

// View renders the current screen.
func (m *Model) View() tea.View {
	v := tea.NewView(m.Render())
	v.AltScreen = true
	v.WindowTitle = appinfo.Name
	return v
}

// Render returns the screen as text: header, body, status and keys. Each
// line is cut to the terminal width with "…" (i shows full text).
func (m *Model) Render() string {
	var b strings.Builder
	b.WriteString(boldStyle.Render(appinfo.Name+" · "+appinfo.Title) + "\n")
	b.WriteString("Profile: " + strings.Join(m.cfg.Profiles, ", ") + "   " + m.breadcrumb() + "\n")
	b.WriteString(m.rule() + "\n")
	b.WriteString(m.body())
	b.WriteString(m.rule() + "\n")
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	b.WriteString(faintStyle.Render(m.keys()))
	return m.fit(b.String())
}

func (m *Model) fit(s string) string {
	if m.width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	return strings.Join(lines, "\n")
}

func (m *Model) rule() string {
	w := m.width
	if w <= 0 || w > 100 {
		w = 100
	}
	return strings.Repeat("─", w)
}

func (m *Model) breadcrumb() string {
	parts := make([]string, len(screenNames))
	for i, n := range screenNames {
		if Screen(i) == m.screen {
			n = "[" + n + "]"
		}
		parts[i] = n
	}
	return strings.Join(parts, " > ")
}

func (m *Model) spinner() string { return spinnerFrames[m.frame%len(spinnerFrames)] }

func (m *Model) body() string {
	switch {
	case m.help:
		return m.helpBody()
	case m.busy != "":
		return m.spinner() + " " + m.busy + "…\n"
	}
	switch m.screen {
	case ScreenPlan:
		if m.info {
			return m.infoBody()
		}
		return m.planBody()
	case ScreenReview:
		return m.reviewBody()
	case ScreenRun:
		return m.runBody()
	case ScreenSummary:
		return m.summaryBody()
	}
	return m.scanBody()
}

func (m *Model) keys() string {
	switch {
	case m.help:
		return "?/esc: close help"
	case m.filtering:
		return "type to filter · enter: keep filter · esc: clear filter"
	case m.info:
		return "i/esc: close"
	case m.screen == ScreenRun:
		if m.confirmStop {
			return "y: stop after the current command · n: keep going"
		}
		return "ctrl+c: stop after the current command · q is ignored while installing"
	case m.busy != "":
		return "q: quit (nothing has changed)"
	}
	switch m.screen {
	case ScreenScan:
		if m.err != nil {
			return "q: quit"
		}
		return "enter: plan · ?: help · q: quit"
	case ScreenPlan:
		return "↑/↓ j/k: move · space: tick · i: info · /: filter · enter: review · esc: back · ?: help · q: quit"
	case ScreenReview:
		if p := m.plan(); p != nil && p.Empty() {
			return "enter: finish (nothing runs) · esc: back · q: quit"
		}
		return "enter: consent and install · esc: back to Plan · ?: help · q: quit (nothing changes)"
	}
	if m.ran {
		return "q: quit"
	}
	return "esc: back · q: quit"
}

func (m *Model) helpBody() string {
	var lines []string
	switch m.screen {
	case ScreenScan:
		lines = []string{"enter      go to the Plan screen"}
	case ScreenPlan:
		lines = []string{
			"↑/↓, k/j   move the cursor",
			"space      tick or untick a missing package; pick a choice member",
			"i          info: source, why it is wanted, which profile asked, conflict evidence",
			"/          filter by name",
			"enter      review the exact commands",
			"esc        back to Scan (your ticks are kept)",
		}
	case ScreenReview:
		lines = []string{
			"enter      consent: run exactly these commands (the only key that consents)",
			"esc        back to Plan (your ticks are kept)",
		}
	case ScreenRun:
		lines = []string{"ctrl+c     stop after the current command (asks first; a sudo step has the terminal and gets ctrl+c itself)"}
	case ScreenSummary:
		lines = []string{"esc        back to Review when nothing ran"}
	}
	lines = append(lines,
		"tab        Manage and Files tabs (a later increment)",
		"?          close this help",
		"q          quit outside Run; nothing changes before consent",
		"ctrl+c     quit outside Run")
	return "Keys on " + m.screen.String() + ":\n\n  " + strings.Join(lines, "\n  ") + "\n"
}

// --- Scan ---

func (m *Model) scanBody() string {
	var b strings.Builder
	res := m.result()
	if res.Bootstrap != nil {
		b.WriteString(machineText(*res.Bootstrap))
	}
	if m.err != nil {
		b.WriteString("\n" + errStyle.Render("facman cannot continue: ") + errorText(m.err) + "\n")
		return b.String()
	}
	var findings []platform.Finding
	if res.Bootstrap != nil {
		for _, f := range res.Bootstrap.Findings {
			if f.Severity != platform.SeverityFatal {
				findings = append(findings, f)
			}
		}
	}
	if res.Scan != nil {
		findings = append(findings, res.Scan.Findings...)
		b.WriteString("\n" + countsText(res.Scan.Statuses) + "\n")
	}
	if len(findings) > 0 {
		b.WriteString("\nFindings:\n")
		for _, f := range findings {
			b.WriteString(findingText(f))
		}
	}
	b.WriteString("\nNothing has changed. Press enter to see what is missing.\n")
	return b.String()
}

func machineText(r platform.BootstrapReport) string {
	p := r.Platform
	where := p.OS + "/" + p.Arch
	switch {
	case p.WSL:
		where += " (WSL)"
	case p.OS == "linux" && !p.Display:
		where += " (no display)"
	}
	if p.Distro.ID != "" {
		where += ", " + p.Distro.ID
	}
	s := "Machine:  " + where + "\n"
	h := r.Homebrew
	switch {
	case !h.Found:
		s += "Homebrew: not installed (Review offers the official installer first)\n"
	case h.OnPATH:
		s += "Homebrew: " + joinNonEmpty(h.Path, h.Version) + "\n"
	default:
		s += "Homebrew: " + h.Path + " (not on PATH; used through brew shellenv)\n"
	}
	if len(h.InheritedEnv) > 0 {
		s += "          brew inherits from your environment: " + strings.Join(h.InheritedEnv, " ") + "\n"
	}
	n := r.Node
	if n.Found {
		s += "Node:     " + joinNonEmpty(n.Version, n.Provenance) + "\n"
	} else {
		s += "Node:     not found\n"
	}
	return s
}

func countsText(sts []ownership.PackageStatus) string {
	counts := map[string]int{}
	total := 0
	for _, st := range sts {
		if !st.Offered && !st.Present() {
			continue
		}
		total++
		counts[stateLabel(st)]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%d %s", counts[k], k)
	}
	return fmt.Sprintf("Scanned %d catalog packages: %s.", total, strings.Join(parts, ", "))
}

func findingText(f platform.Finding) string {
	s := "  " + string(f.Severity) + ": " + f.Message + "\n"
	if f.Fix != "" {
		s += "    fix: " + f.Fix + "\n"
	}
	return s
}

// errorText is the message, the details and the fix of err.
func errorText(err error) string {
	var fe *failure.Error
	if errors.As(err, &fe) {
		s := fe.Error()
		if fe.Fix != "" {
			s += "\n  fix: " + fe.Fix
		}
		return s
	}
	return err.Error()
}

// --- Plan ---

func (m *Model) planBody() string {
	rows := m.visibleRows()
	var b strings.Builder
	selected := 0
	for _, r := range m.rows() {
		if r.selectable && r.checked {
			selected++
		}
	}
	fmt.Fprintf(&b, "Tick what to install (%d ticked). Required missing packages start ticked; optional ones do not.\n", selected)
	if m.filtering || m.filter != "" {
		b.WriteString("/ " + m.filter)
		if m.filtering {
			b.WriteString("_")
		}
		b.WriteString("\n")
	}
	if len(rows) == 0 {
		b.WriteString("\nNo package matches.\n")
		return b.String()
	}
	lines := make([]string, 0, len(rows))
	cursorLine := 0
	for i, r := range rows {
		if i == m.cursor {
			cursorLine = len(lines)
		}
		lines = append(lines, m.rowLine(r, i == m.cursor))
	}
	b.WriteString(strings.Join(m.window(lines, cursorLine), "\n") + "\n")
	return b.String()
}

// window keeps the lines around the cursor that fit the terminal.
func (m *Model) window(lines []string, cursor int) []string {
	room := m.height - chromeLines - 2
	if m.height <= 0 || room <= 0 || len(lines) <= room {
		m.offset = 0
		return lines
	}
	if cursor < m.offset {
		m.offset = cursor
	}
	if cursor >= m.offset+room {
		m.offset = cursor - room + 1
	}
	end := min(m.offset+room, len(lines))
	return lines[m.offset:end]
}

func (m *Model) rowLine(r row, cursor bool) string {
	if r.kind == rowHeader {
		return boldStyle.Render(r.text)
	}
	mark := "  "
	if cursor {
		mark = "> "
	}
	box := "   "
	open, shut := "[", "]"
	if r.kind == rowMember {
		open, shut = "(", ")"
	}
	if r.box {
		switch {
		case r.greyed:
			box = open + "-" + shut
		case r.checked:
			box = open + "x" + shut
		default:
			box = open + " " + shut
		}
	}
	name := r.id
	if r.kind == rowMember {
		name = "  " + name
	}
	line := fmt.Sprintf("%s%s %-24s %-16s %s", mark, box, name, r.state, r.note)
	line = strings.TrimRight(line, " ")
	if r.greyed {
		return faintStyle.Render(line)
	}
	return line
}

// infoBody is i on Plan: source, why it is wanted, which profile asked,
// detections and conflict evidence, in full (wrapped, not cut).
func (m *Model) infoBody() string {
	r, ok := cursorRow(m.visibleRows(), m.cursor)
	res := m.result()
	if !ok || res.Catalog == nil || res.Scan == nil {
		return ""
	}
	e, _ := res.Catalog.Entry(r.id)
	st, _ := res.Scan.Status(r.id)
	var b strings.Builder
	title := r.id
	if e.Display != "" {
		title = e.Display + " (" + r.id + ")"
	}
	b.WriteString(boldStyle.Render(title) + "\n")
	fmt.Fprintf(&b, "State:   %s\n", stateLabel(st))
	if res.Bootstrap != nil {
		b.WriteString("Source:  " + sourceText(e, res.Bootstrap.Platform) + "\n")
	}
	b.WriteString("Why:     " + m.whyText(r) + "\n")
	if st.UnavailableReason != "" || st.Hint != "" {
		b.WriteString("Reason:  " + joinNonEmpty(st.UnavailableReason, st.Hint) + "\n")
	}
	if st.Incomplete {
		b.WriteString("Note:    " + incompleteReason(st) + "\n")
	}
	if len(st.Detections) > 0 {
		b.WriteString("Found:\n")
		for _, d := range st.Detections {
			if d.Found {
				fmt.Fprintf(&b, "  %s %s %s %s\n", d.Detector, d.Provenance, d.Version, d.Path)
			} else if d.Status == ownership.DetectionUnknown {
				fmt.Fprintf(&b, "  %s could not answer: %s\n", d.Detector, d.Reason)
			}
		}
	}
	if c := st.Conflict; c != nil {
		b.WriteString(conflictText(c))
	}
	return m.wrap(b.String())
}

func (m *Model) wrap(s string) string {
	if m.width <= 0 {
		return s
	}
	return ansi.Wrap(s, m.width, " ")
}

func (m *Model) whyText(r row) string {
	if r.kind == rowMember {
		return "member of the choice group " + r.group
	}
	it, ok := m.desired.Item(r.id)
	if !ok {
		return "not wanted by the selected profiles"
	}
	var parts []string
	switch {
	case it.Required:
		parts = append(parts, "required by profile "+strings.Join(it.FromProfiles, ", "))
	case len(it.FromProfiles) > 0:
		parts = append(parts, "optional in profile "+strings.Join(it.FromProfiles, ", "))
	}
	if len(it.RequiredBy) > 0 {
		parts = append(parts, "needed by "+strings.Join(it.RequiredBy, ", "))
	}
	if it.Version != "" {
		parts = append(parts, "pinned to "+it.Version)
	}
	return strings.Join(parts, "; ")
}

func sourceText(e catalog.Entry, p platform.Platform) string {
	steps := e.Install[p.Key()]
	if len(steps) == 0 {
		return "no install step for " + string(p.Key())
	}
	var parts []string
	for _, s := range steps {
		t := string(s.Backend) + " " + s.ID
		if s.Tap != "" {
			t += " (tap " + s.Tap
			if s.Trust == catalog.TrustItem {
				t += ", item-level trust"
			}
			t += ")"
		}
		parts = append(parts, strings.TrimSpace(t))
	}
	return strings.Join(parts, ", then ")
}

func conflictText(c *plan.Conflict) string {
	var b strings.Builder
	b.WriteString("Conflict:\n")
	fmt.Fprintf(&b, "  path:    %s\n", c.Path)
	fmt.Fprintf(&b, "  owner:   %s\n", joinNonEmpty(string(c.Owner), c.OwnerDetail))
	ev := c.Evidence
	if ev.LsLine != "" {
		fmt.Fprintf(&b, "  ls -l:   %s\n", ev.LsLine)
	}
	if ev.FileType != "" {
		fmt.Fprintf(&b, "  type:    %s\n", ev.FileType)
	}
	if ev.IsSymlink {
		target := ev.SymlinkTarget
		if ev.Dangling {
			target += " (dangling)"
		}
		fmt.Fprintf(&b, "  link to: %s\n", target)
	}
	if len(c.FailingCommand) > 0 {
		fmt.Fprintf(&b, "  would fail: %s\n", plan.Command{Argv: c.FailingCommand}.String())
	}
	if c.FixHint != "" {
		fmt.Fprintf(&b, "  fix:     %s\n", c.FixHint)
	}
	return b.String()
}

// --- Review ---

func (m *Model) plan() *plan.Plan {
	return m.result().Plan
}

// runsBrew reports whether any command of p starts with a bare "brew".
func runsBrew(p *plan.Plan) bool {
	for _, c := range p.Commands() {
		if len(c.Argv) > 0 && c.Argv[0] == "brew" {
			return true
		}
	}
	return false
}

func (m *Model) reviewBody() string {
	p := m.plan()
	if p == nil {
		return ""
	}
	var b strings.Builder
	for _, f := range p.Warnings {
		b.WriteString(findingText(f))
	}
	if len(p.Warnings) > 0 {
		b.WriteString("\n")
	}
	if p.Empty() {
		b.WriteString("Nothing to install.\n")
	} else {
		if bs := m.result().Bootstrap; bs != nil && bs.Homebrew.Found && runsBrew(p) {
			// The plan names brew; this is the one it runs (review FR-012).
			b.WriteString("Homebrew: " + plan.Command{Argv: []string{bs.Homebrew.Path}}.String() + " (every brew command below runs this one)\n")
			if env := bs.Homebrew.InheritedEnv; len(env) > 0 {
				b.WriteString("          and inherits from your environment: " + strings.Join(env, " ") + "\n")
			}
		}
		fmt.Fprintf(&b, "These %d command(s) run in this order, and nothing else. Nothing has run yet.\n\n", len(p.Commands()))
		for _, a := range p.Actions {
			for _, c := range a.Commands {
				fmt.Fprintf(&b, "  %2d. %-22s %s\n", a.ID, a.PackageID, c.String())
				if marks := commandMarks(a, c); marks != "" {
					b.WriteString("      " + marks + "\n")
				}
			}
		}
	}
	if len(p.Excluded) > 0 {
		b.WriteString("\nNot installed:\n")
		for _, ex := range p.Excluded {
			fmt.Fprintf(&b, "  %s: %s\n", ex.PackageID, joinNonEmpty(string(ex.Reason), ex.Detail))
		}
	}
	if !p.Empty() {
		user := m.cfg.User
		if user == "" {
			user = "your OS user"
		}
		fmt.Fprintf(&b, "\nPressing enter is your consent. It is recorded as %s with the time.\n", user)
	}
	return b.String()
}

// commandMarks says what Review must show about a command: sudo, an
// unverified script, item-level trust and the dependency order.
func commandMarks(a plan.Action, c plan.Command) string {
	var marks []string
	if c.NeedsSudo {
		marks = append(marks, "may ask for your sudo password: the terminal is handed to it")
	}
	if c.Unverified {
		marks = append(marks, "unverified script: Homebrew publishes no checksum")
	}
	if a.Kind == plan.ActionTap && a.TapTrust != nil {
		marks = append(marks, "adds the third-party tap "+a.TapTrust.Tap)
	}
	if a.Kind == plan.ActionTrust && a.TapTrust != nil {
		marks = append(marks, "trusts only "+a.TapTrust.Item+", not the whole tap")
	}
	if len(a.DependsOn) > 0 {
		marks = append(marks, "after "+idList(a.DependsOn))
	}
	if len(marks) == 0 {
		return ""
	}
	return "[" + strings.Join(marks, "; ") + "]"
}

func idList(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return strings.Join(parts, ", ")
}

// --- Run ---

func (m *Model) runBody() string {
	p := m.plan()
	if p == nil {
		return ""
	}
	var b strings.Builder
	done := len(m.results)
	fmt.Fprintf(&b, "Installing: %d of %d actions done.\n\n", done, len(p.Actions))
	for _, a := range p.Actions {
		mark := "      "
		if r, ok := m.results[a.ID]; ok {
			mark = resultMark(r.Status)
		} else if a.ID == m.current {
			mark = "  " + m.spinner() + "   "
		}
		cmds := make([]string, len(a.Commands))
		for i, c := range a.Commands {
			cmds[i] = c.String()
		}
		fmt.Fprintf(&b, "%s %2d. %-22s %s\n", mark, a.ID, a.PackageID, strings.Join(cmds, "; "))
	}
	if m.handingOff != "" {
		b.WriteString("\nThe terminal is with: " + m.handingOff + "\n")
	}
	if tail := m.tail(); len(tail) > 0 {
		b.WriteString("\nLog:\n")
		for _, l := range tail {
			b.WriteString("  " + l + "\n")
		}
	}
	switch {
	case m.confirmStop:
		b.WriteString("\nStop after the current command? The command that is running is never killed. [y/N]\n")
	case m.stopping:
		b.WriteString("\nStopping after the current command…\n")
	}
	return b.String()
}

func (m *Model) tail() []string {
	n := 6
	if len(m.logTail) < n {
		n = len(m.logTail)
	}
	return m.logTail[len(m.logTail)-n:]
}

func resultMark(s execute.ActionStatus) string {
	switch s {
	case execute.ActionSucceeded:
		return "  ok  "
	case execute.ActionFailed:
		return "FAILED"
	case execute.ActionSkipped:
		return " skip "
	case execute.ActionAlreadyPresent:
		return " had  "
	}
	return " stop "
}

// --- Summary ---

func (m *Model) summaryBody() string {
	res := m.result()
	var b strings.Builder
	s := res.Summary
	if s == nil {
		if m.err != nil {
			b.WriteString(errStyle.Render("Not everything happened: ") + errorText(m.err) + "\n")
		} else {
			b.WriteString("Nothing was changed.\n")
		}
		return b.String()
	}
	if len(s.Lines) == 0 {
		b.WriteString("Nothing to install: everything wanted is already here.\n")
	}
	for _, l := range s.Lines {
		b.WriteString("  " + l.String() + "\n")
		if l.LogPath != "" && !l.Installed {
			b.WriteString("    log: " + l.LogPath + "\n")
		}
	}
	if len(s.NextSteps) > 0 {
		b.WriteString("\nNext steps (installed, sign-in required):\n")
		for _, n := range s.NextSteps {
			b.WriteString("  " + n + "\n")
		}
	}
	if len(s.ProfileLines) > 0 {
		b.WriteString("\nAdd to your shell profile:\n")
		for _, l := range s.ProfileLines {
			b.WriteString("  " + l + "\n")
		}
	}
	if res.Outcome != nil {
		for _, w := range res.Outcome.Warnings {
			b.WriteString("warning: " + w + "\n")
		}
		if logs := logPaths(res.Outcome.Results); len(logs) > 0 {
			b.WriteString("\nLogs:\n")
			for _, l := range logs {
				b.WriteString("  " + l + "\n")
			}
		}
	}
	if m.err != nil {
		b.WriteString("\n" + errStyle.Render(fmt.Sprintf("facman exits %d: ", failure.ExitCode(m.err))) + errorText(m.err) + "\n")
	}
	return b.String()
}

func logPaths(rs []execute.ActionResult) []string {
	var out []string
	for _, r := range rs {
		if r.LogPath != "" {
			out = append(out, r.LogPath)
		}
	}
	return out
}

func commandLine(c process.Cmd) string {
	return plan.Command{Argv: c.Argv}.String()
}

func (m *Model) result() sessionResult { return m.res }
