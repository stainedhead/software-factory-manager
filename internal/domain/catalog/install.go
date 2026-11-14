package catalog

import (
	"fmt"

	"github.com/stainedhead/software-factory-manager/internal/domain/platform"
)

// BackendSet is the set of install backends this build implements.
type BackendSet map[BackendID]bool

// Has reports whether b is implemented.
func (s BackendSet) Has(b BackendID) bool { return s[b] }

// Availability is the result of SelectInstall for one entry on one platform.
type Availability struct {
	// Offered is false when the entry is not meant for this platform at all
	// (platforms list excludes it, or no install block): it is not shown as
	// wanted.
	Offered bool
	// Available is true when Step can be installed here.
	Available bool
	// Step is the chosen install step when Available.
	Step *InstallStep
	// Reason says why an offered entry is unavailable here.
	Reason string
	// Hint is the one-line hint for an unavailable entry.
	Hint string
}

// Unavailable reports the unavailable-here case: offered but not installable.
func (a Availability) Unavailable() bool { return a.Offered && !a.Available }

// backendIncrement names when an unimplemented backend arrives.
var backendIncrement = map[BackendID]string{BackendNPM: "Increment 2"}

// SelectInstall chooses how entry is installed on p, given the backends this
// build implements (FR-043, FR-044, FR-045). Rules, in order:
//   - a platforms list that excludes p: not offered;
//   - choice groups: offered and available, with no step (members decide);
//   - the install block for p's key; on WSL a cli entry with no wsl block
//     inherits linux, and a gui entry with no wsl block is a host item
//     (unavailable-here, with a winget hint when the catalog has one);
//   - no block: not offered;
//   - a gui entry on Linux without a display: unavailable-here;
//   - the first step whose backend is implemented wins; report and
//     host-report steps make the entry unavailable-here with their hint;
//     an unimplemented backend gives "needs the <backend> backend".
func SelectInstall(entry Entry, p platform.Platform, implemented BackendSet) Availability {
	key := p.Key()
	if !entry.OfferedOn(key) {
		return Availability{}
	}
	if entry.IsChoice() {
		return Availability{Offered: true, Available: true}
	}
	steps, ok := entry.Install[key]
	if !ok && key == platform.WSL {
		if entry.Kind == KindGUI {
			return Availability{Offered: true, Reason: "GUI apps live on the Windows host", Hint: wingetHint(entry, "")}
		}
		steps, ok = entry.Install[platform.Linux]
	}
	if !ok || len(steps) == 0 {
		return Availability{}
	}
	if entry.Kind == KindGUI && key == platform.Linux && !p.Display {
		return Availability{Offered: true, Reason: "no display: GUI apps are skipped on headless Linux"}
	}
	var missing *InstallStep
	for i := range steps {
		s := steps[i]
		switch {
		case s.Backend.ReportOnly():
			hint := s.Hint
			if s.Backend == BackendHostReport {
				hint = wingetHint(entry, s.Hint)
			}
			return Availability{Offered: true, Reason: reportReason(s.Backend), Hint: hint}
		case implemented.Has(s.Backend):
			return Availability{Offered: true, Available: true, Step: &s}
		case missing == nil:
			missing = &s
		}
	}
	return Availability{Offered: true, Reason: needsBackend(missing.Backend), Hint: missing.Hint}
}

func reportReason(b BackendID) string {
	if b == BackendHostReport {
		return "install it on the Windows host"
	}
	return "not installable through Homebrew on this platform"
}

func needsBackend(b BackendID) string {
	if inc, ok := backendIncrement[b]; ok {
		return fmt.Sprintf("needs the %s backend (%s)", b, inc)
	}
	return fmt.Sprintf("needs the %s backend, which this version does not provide", b)
}

func wingetHint(e Entry, hint string) string {
	if hint != "" {
		return hint
	}
	if e.Detect.WingetID != "" {
		return "Install on the Windows host: winget install " + e.Detect.WingetID
	}
	return "Install it on the Windows host"
}
