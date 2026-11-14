package statedir

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/failure"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{"macOS", "darwin", map[string]string{"HOME": "/Users/j", "XDG_STATE_HOME": "/x"}, "/Users/j/Library/Application Support/facman", false},
		{"linux default", "linux", map[string]string{"HOME": "/home/j"}, "/home/j/.local/state/facman", false},
		{"linux XDG", "linux", map[string]string{"HOME": "/home/j", "XDG_STATE_HOME": "/var/state"}, "/var/state/facman", false},
		{"relative XDG ignored", "linux", map[string]string{"HOME": "/home/j", "XDG_STATE_HOME": "rel"}, "/home/j/.local/state/facman", false},
		{"XDG without HOME", "linux", map[string]string{"XDG_STATE_HOME": "/var/state"}, "/var/state/facman", false},
		{"HOME unset", "linux", nil, "", true},
		{"HOME relative", "darwin", map[string]string{"HOME": "home"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.goos, env(tt.env))
			if tt.wantErr {
				if failure.ExitCode(err) != 4 || !strings.Contains(err.Error(), "HOME") {
					t.Errorf("err = %v, want a bootstrap error naming HOME", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("Resolve = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestOpenAndPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state", "facman")
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(d.LogsPath()); err != nil || !fi.IsDir() {
		t.Fatalf("logs dir: %v", err)
	}
	if d.Root() != root || d.DBPath() != filepath.Join(root, "state.db") || d.LockPath() != filepath.Join(root, "facman.lock") {
		t.Errorf("paths: %s %s %s", d.Root(), d.DBPath(), d.LockPath())
	}
	at := time.Date(2026, 11, 14, 14, 0, 0, 0, time.FixedZone("EST", -5*3600))
	if got := filepath.Base(d.LogPath(7, "gh", at)); got != "20261114-190000-7-gh.log" {
		t.Errorf("log name = %s", got)
	}
	if filepath.Dir(d.LogPath(7, "gh", at)) != d.LogsPath() {
		t.Error("log not under logs/")
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".write-check") {
			t.Errorf("write check left %s behind", e.Name())
		}
	}
}

func TestOpenUnwritable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(filepath.Join(file, "facman"))
	if failure.ExitCode(err) != 4 || !strings.Contains(err.Error(), file) {
		t.Errorf("err = %v, want exit 4 naming the path", err)
	}
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere; the read-only directory case needs a normal user")
	}
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(filepath.Join(ro, LogsDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	if _, err := Open(ro); failure.ExitCode(err) != 4 || !strings.Contains(err.Error(), "not writable") {
		t.Errorf("read-only err = %v", err)
	}
}

func TestPruneLogs(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 14, 19, 0, 0, 0, time.UTC)
	write := func(path string, age time.Duration) {
		t.Helper()
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	old := filepath.Join(d.LogsPath(), "old.log")
	fresh := filepath.Join(d.LogsPath(), "fresh.log")
	outside := filepath.Join(d.Root(), "old-outside.log")
	write(old, 91*24*time.Hour)
	write(fresh, 89*24*time.Hour)
	write(outside, 400*24*time.Hour)
	if err := os.Mkdir(filepath.Join(d.LogsPath(), "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := d.PruneLogs(now); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{old: false, fresh: true, outside: true, filepath.Join(d.LogsPath(), "subdir"): true} {
		if _, err := os.Stat(path); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", path, err == nil, want)
		}
	}
	if err := os.RemoveAll(d.LogsPath()); err != nil {
		t.Fatal(err)
	}
	if err := d.PruneLogs(now); err == nil {
		t.Error("pruning a missing logs dir should fail")
	}
}

const helperEnv = "STATEDIR_LOCK_HELPER"

// TestHelperHoldLock holds the lock in a child process until stdin closes.
func TestHelperHoldLock(t *testing.T) {
	root := os.Getenv(helperEnv)
	if root == "" {
		t.Skip("helper process only")
	}
	d, err := Open(root)
	if err != nil {
		os.Exit(2)
	}
	unlock, _, err := d.TryLock()
	if err != nil || unlock == nil {
		os.Exit(3)
	}
	_, _ = os.Stdout.WriteString("locked\n")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	unlock()
	os.Exit(0)
}

func TestLockAcrossProcesses(t *testing.T) {
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=TestHelperHoldLock")
	child.Env = append(os.Environ(), helperEnv+"="+root)
	stdin, _ := child.StdinPipe()
	stdout, _ := child.StdoutPipe()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = child.Wait() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("child did not lock: %q %v", line, err)
	}

	unlock, holder, err := d.TryLock()
	if err != nil || unlock != nil {
		t.Fatalf("TryLock while held = %v, %v", unlock != nil, err)
	}
	if holder != child.Process.Pid {
		t.Errorf("holder = %d, want child PID %d", holder, child.Process.Pid)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if _, err := d.Lock(ctx); failure.ExitCode(err) != 4 || !strings.Contains(err.Error(), "PID "+strconv.Itoa(child.Process.Pid)) {
		t.Errorf("Lock while held: %v", err)
	}

	// release in the child; Lock then succeeds
	_ = stdin.Close()
	_ = child.Wait()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	unlock, err = d.Lock(ctx2)
	if err != nil || unlock == nil {
		t.Fatalf("Lock after release: %v", err)
	}
	b, _ := os.ReadFile(d.LockPath())
	if strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
		t.Errorf("lock file = %q, want our PID", b)
	}
	unlock()
	if b, _ := os.ReadFile(d.LockPath()); len(b) != 0 {
		t.Errorf("lock file after unlock = %q", b)
	}
}

func TestLockOpenFailure(t *testing.T) {
	d := &Dir{root: filepath.Join(t.TempDir(), "missing")}
	if _, _, err := d.TryLock(); failure.ExitCode(err) != 4 {
		t.Errorf("TryLock err = %v", err)
	}
	if _, err := d.Lock(context.Background()); failure.ExitCode(err) != 4 {
		t.Errorf("Lock err = %v", err)
	}
}

func TestReadPIDGarbage(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "lock")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString("not-a-pid")
	if readPID(f) != 0 {
		t.Error("garbage PID should read as 0")
	}
}
