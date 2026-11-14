// Package statedir owns facman's state directory (PRD Appendix J): its
// location per platform, the SQLite file path, per-operation log names,
// 90-day log pruning and the exclusive flock that serializes mutating runs.
package statedir

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/appinfo"
	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
	"github.com/stainedhead/software-factory-manager/internal/domain/inventory"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
	"github.com/stainedhead/software-factory-manager/internal/usecase/scan"
)

// File names inside the state directory.
const (
	DBFile   = "state.db"
	LockFile = appinfo.Name + ".lock"
	LogsDir  = "logs"
)

// LogRetention is how long operation logs are kept.
const LogRetention = 90 * 24 * time.Hour

// lockPoll is how often Lock retries while another process holds the lock.
const lockPoll = 100 * time.Millisecond

// Resolve returns the state directory for goos:
// macOS ~/Library/Application Support/facman, Linux and WSL
// ${XDG_STATE_HOME:-~/.local/state}/facman. HOME unset (or relative) is a
// bootstrap error naming the variable.
func Resolve(goos string, getenv func(string) string) (string, error) {
	if goos != "darwin" {
		if x := getenv("XDG_STATE_HOME"); x != "" && filepath.IsAbs(x) {
			return filepath.Join(x, appinfo.Name), nil
		}
	}
	home := getenv("HOME")
	if home == "" || !filepath.IsAbs(home) {
		return "", failure.Bootstrap("HOME is not set to an absolute path, so the state directory cannot be found").
			WithFix("set HOME to your home directory and run again")
	}
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support", appinfo.Name), nil
	}
	return filepath.Join(home, ".local", "state", appinfo.Name), nil
}

// Dir is an opened state directory. It implements the StateDir ports.
type Dir struct {
	root string
	pid  int
}

var (
	_ scan.StateDir    = (*Dir)(nil)
	_ execute.StateDir = (*Dir)(nil)
)

// Open creates root and root/logs (0700) if needed and checks that root is
// writable. Any failure is a bootstrap error (exit 4) naming the path.
func Open(root string) (*Dir, error) {
	for _, d := range []string{root, filepath.Join(root, LogsDir)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, failure.Bootstrap("state directory %s cannot be created: %w", d, err).
				WithFix("make the directory writable by your user, or free disk space")
		}
	}
	probe, err := os.CreateTemp(root, ".write-check-*")
	if err != nil {
		return nil, failure.Bootstrap("state directory %s is not writable: %w", root, err).
			WithFix("make the directory writable by your user, or free disk space")
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return &Dir{root: root, pid: os.Getpid()}, nil
}

// Root returns the state directory.
func (d *Dir) Root() string { return d.root }

// DBPath returns the SQLite file.
func (d *Dir) DBPath() string { return filepath.Join(d.root, DBFile) }

// LockPath returns the lock file.
func (d *Dir) LockPath() string { return filepath.Join(d.root, LockFile) }

// LogsPath returns the log directory.
func (d *Dir) LogsPath() string { return filepath.Join(d.root, LogsDir) }

// LogPath returns logs/<YYYYMMDD-HHMMSS>-<opID>-<pkgID>.log, with t in UTC.
// Every byte of pkgID outside [A-Za-z0-9._+@-] becomes "_", so the file is
// always directly inside logs/ (review FR-001d; catalog IDs are validated
// at load, this is defence in depth).
func (d *Dir) LogPath(opID int64, pkgID string, t time.Time) string {
	name := fmt.Sprintf("%s-%d-%s.log", t.UTC().Format("20060102-150405"), opID, logSafe(pkgID))
	return filepath.Join(d.LogsPath(), name)
}

// logSafe replaces every byte outside [A-Za-z0-9._+@-] with "_". With no
// separator left, the joined name cannot leave logs/ ("." and ".." are
// impossible because the name always has the time prefix).
func logSafe(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.IndexByte("._+@-", c) >= 0:
		default:
			b[i] = '_'
		}
	}
	return string(b)
}

// OpenLog opens the log at path for appending (0600), creating its
// directory (0700) when needed.
func (d *Dir) OpenLog(path string) (io.WriteCloser, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating the log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening log %s: %w", path, err)
	}
	return f, nil
}

// PruneLogs deletes regular files in logs/ last modified more than 90 days
// before now. Nothing outside logs/ is touched; rows keep their log path.
func (d *Dir) PruneLogs(now time.Time) error {
	entries, err := os.ReadDir(d.LogsPath())
	if err != nil {
		return fmt.Errorf("listing %s: %w", d.LogsPath(), err)
	}
	var errs []error
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // removed meanwhile
		}
		if now.Sub(info.ModTime()) > LogRetention {
			if err := os.Remove(filepath.Join(d.LogsPath(), e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("pruning logs: %w", errors.Join(errs...))
	}
	return nil
}

// TryLock takes the exclusive lock without waiting. When another process
// holds it, unlock is nil and holder is that process's PID (0 if unknown;
// messages show it with inventory.HolderLabel).
func (d *Dir) TryLock() (unlock func(), holder int, err error) {
	f, err := os.OpenFile(d.LockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, 0, failure.Bootstrap("opening lock %s: %w", d.LockPath(), err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		holder := readPID(f)
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, holder, nil
		}
		return nil, 0, failure.Bootstrap("locking %s: %w", d.LockPath(), err)
	}
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteAt([]byte(strconv.Itoa(d.pid)+"\n"), 0)
	}
	return func() {
		_ = f.Truncate(0)
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, 0, nil
}

// Lock takes the exclusive lock, waiting until ctx is done.
func (d *Dir) Lock(ctx context.Context) (func(), error) {
	for {
		unlock, holder, err := d.TryLock()
		if err != nil || unlock != nil {
			return unlock, err
		}
		select {
		case <-ctx.Done():
			return nil, failure.Bootstrap("another %s run (%s) holds %s: %w", appinfo.Name, inventory.HolderLabel(holder), d.LockPath(), ctx.Err()).
				WithFix("wait for it to finish, then run again")
		case <-time.After(lockPoll):
		}
	}
}

func readPID(f *os.File) int {
	buf := make([]byte, 32)
	n, _ := f.ReadAt(buf, 0)
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		return 0
	}
	return pid
}
