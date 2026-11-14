// Package fsprobe inspects a path a backend would write (FR-015, FR-016):
// what sits there (an ls -l style line, file type, symlink target and
// whether it dangles, executable bit, size, modification time) and who
// owns it (this package, another brew item, npm, another installer, or
// nobody). It only reads the filesystem and runs `file -b` when present.
package fsprobe

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/catalog"
	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/conflicts"
	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

// fileTimeout bounds the `file -b` call.
const fileTimeout = 5 * time.Second

// Runner runs `file -b`. Implemented by adapter/execrunner.
type Runner interface {
	Run(ctx context.Context, cmd process.Cmd) (process.Result, error)
}

// Options configure a Probe.
type Options struct {
	// Locations returns the current Homebrew prefix, npm prefix and home
	// (they change when facman installs Homebrew); required.
	Locations func() catalog.Locations
	// FileCmd names the file program; default "file". Its absence is fine.
	FileCmd string
	// Env are the environment overrides for running FileCmd.
	Env func() map[string]string
}

// Probe implements conflicts.PathProbe.
type Probe struct {
	runner Runner
	opts   Options
}

var _ conflicts.PathProbe = (*Probe)(nil)

// New returns a Probe.
func New(runner Runner, opts Options) *Probe {
	if opts.FileCmd == "" {
		opts.FileCmd = "file"
	}
	if opts.Env == nil {
		opts.Env = func() map[string]string { return nil }
	}
	return &Probe{runner: runner, opts: opts}
}

// Inspect returns what is at path, or nil when nothing is there.
func (p *Probe) Inspect(ctx context.Context, path string) (*plan.ConflictEvidence, error) {
	lfi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspecting %s: %w", path, err)
	}
	ev := &plan.ConflictEvidence{Size: lfi.Size(), ModTime: lfi.ModTime()}
	mode := lfi.Mode()
	if mode&fs.ModeSymlink != 0 {
		ev.IsSymlink = true
		if ev.SymlinkTarget, err = os.Readlink(path); err != nil {
			return nil, fmt.Errorf("reading link %s: %w", path, err)
		}
		if tfi, err := os.Stat(path); err != nil {
			ev.Dangling = true
		} else {
			ev.Executable = !tfi.IsDir() && tfi.Mode().Perm()&0o111 != 0
		}
	} else {
		ev.Executable = !lfi.IsDir() && mode.Perm()&0o111 != 0
	}
	ev.LsLine = lsLine(path, lfi, ev.SymlinkTarget)
	ev.FileType = p.fileType(ctx, path, lfi, ev)
	return ev, nil
}

// fileType runs `file -b`, falling back to a type derived in Go.
func (p *Probe) fileType(ctx context.Context, path string, fi fs.FileInfo, ev *plan.ConflictEvidence) string {
	res, err := p.runner.Run(ctx, process.Cmd{Argv: []string{p.opts.FileCmd, "-b", path}, Env: p.opts.Env(), Timeout: fileTimeout})
	if err == nil && res.Success() {
		if out := strings.TrimSpace(res.Stdout); out != "" {
			return out
		}
	}
	switch {
	case ev.IsSymlink && ev.Dangling:
		return "broken symbolic link to " + ev.SymlinkTarget
	case ev.IsSymlink:
		return "symbolic link to " + ev.SymlinkTarget
	case fi.IsDir():
		return "directory"
	case fi.Size() == 0:
		return "empty"
	case ev.Executable:
		return "executable"
	default:
		return "regular file"
	}
}

// lsLine renders an ls -l style line: mode, links, owner, group, size,
// modification time (UTC), path and link target.
func lsLine(path string, fi fs.FileInfo, target string) string {
	nlink, owner, group := "1", "?", "?"
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		nlink = fmt.Sprint(st.Nlink) // uint64 on Linux, uint16 on macOS
		owner, group = userName(st.Uid), groupName(st.Gid)
	}
	line := fmt.Sprintf("%s %s %s %s %d %s %s", modeString(fi.Mode()), nlink, owner, group, fi.Size(),
		fi.ModTime().UTC().Format("Jan _2 15:04 2006"), path)
	if target != "" {
		line += " -> " + target
	}
	return line
}

func modeString(m fs.FileMode) string {
	t := "-"
	switch {
	case m&fs.ModeSymlink != 0:
		t = "l"
	case m.IsDir():
		t = "d"
	}
	return t + m.Perm().String()[1:]
}

func userName(uid uint32) string {
	id := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(id); err == nil {
		return u.Username
	}
	return id
}

func groupName(gid uint32) string {
	id := strconv.FormatUint(uint64(gid), 10)
	if g, err := user.LookupGroupId(id); err == nil {
		return g.Name
	}
	return id
}

// Owner classifies who owns the file at path for entry. A dangling symlink
// is an orphan. Otherwise the path, resolved through symlinks, decides:
// {prefix}/Caskroom/<token> and {prefix}/Cellar|opt/<name> belong to this
// package when the entry installs that token or name, else to another brew
// item; .../lib/node_modules/<pkg> under the brew or npm prefix belongs to
// npm; a path matching one of the entry's other provenance globs, or an
// OS directory, belongs to another installer; anything else is an orphan.
func (p *Probe) Owner(_ context.Context, path string, entry catalog.Entry) (plan.OwnerClass, string, error) {
	lfi, err := os.Lstat(path)
	if err != nil {
		return "", "", fmt.Errorf("checking owner of %s: %w", path, err)
	}
	resolved := path
	if lfi.Mode()&fs.ModeSymlink != 0 {
		target, _ := os.Readlink(path)
		r, err := filepath.EvalSymlinks(path)
		if err != nil {
			return plan.OwnerOrphan, "dangling symlink to " + target + "; no package manager owns it", nil
		}
		resolved = r
	}
	loc := p.opts.Locations()
	if class, detail, ok := brewOwner(resolved, loc.BrewPrefix, entry); ok {
		return class, detail, nil
	}
	for _, prefix := range []string{loc.BrewPrefix, loc.NPMPrefix} {
		if pkg, ok := npmPackage(resolved, prefix); ok {
			return plan.OwnerNPM, "npm " + pkg, nil
		}
	}
	switch prov := catalog.ProvenanceOf(entry, resolved, loc); prov {
	case catalog.ProvUnknown, catalog.ProvBrew, catalog.ProvBrewCask, catalog.ProvNPM:
	case catalog.ProvOS:
		return plan.OwnerOtherInstaller, "the operating system (" + resolved + ")", nil
	default:
		return plan.OwnerOtherInstaller, string(prov) + " install (" + resolved + ")", nil
	}
	return plan.OwnerOrphan, "no package manager owns it", nil
}

// brewOwner classifies paths inside the Homebrew prefix's Caskroom,
// Cellar and opt directories.
func brewOwner(resolved, prefix string, e catalog.Entry) (plan.OwnerClass, string, bool) {
	if prefix == "" {
		return "", "", false
	}
	for _, area := range []struct {
		dir, kind string
		backend   catalog.BackendID
	}{
		{"Caskroom", "cask", catalog.BackendBrewCask},
		{"Cellar", "formula", catalog.BackendBrewFormula},
		{"opt", "formula", catalog.BackendBrewFormula},
	} {
		name, ok := firstSegment(resolved, filepath.Join(prefix, area.dir))
		if !ok {
			continue
		}
		for _, steps := range e.Install {
			for _, s := range steps {
				if s.Backend == area.backend && lastSegment(s.ID) == name {
					return plan.OwnerThisPackage, "brew " + area.kind + " " + s.ID, true
				}
			}
		}
		return plan.OwnerOtherBrew, "brew " + area.kind + " " + name, true
	}
	return "", "", false
}

func npmPackage(resolved, prefix string) (string, bool) {
	if prefix == "" {
		return "", false
	}
	first, ok := firstSegment(resolved, filepath.Join(prefix, "lib", "node_modules"))
	if !ok {
		return "", false
	}
	if strings.HasPrefix(first, "@") {
		rest := strings.TrimPrefix(resolved, filepath.Join(prefix, "lib", "node_modules", first)+"/")
		if second, _, _ := strings.Cut(rest, "/"); second != "" && rest != resolved {
			return first + "/" + second, true
		}
	}
	return first, true
}

// firstSegment returns the first path element of p below dir.
func firstSegment(p, dir string) (string, bool) {
	rest, ok := strings.CutPrefix(p, strings.TrimSuffix(dir, "/")+"/")
	if !ok || rest == "" {
		return "", false
	}
	seg, _, _ := strings.Cut(rest, "/")
	return seg, true
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}
