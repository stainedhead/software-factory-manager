// Command fakecmd is a fake multi-call executable for tests. Linked or
// copied as brew, xcode-select, mdfind, plutil, npm, getconf, ldd or file, it
// records every call and answers from the world file next to it (see
// internal/testkit/fakeworld). It is never part of the product: Go ignores
// testdata for ./..., and only internal/testkit builds it.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/testkit/fakeworld"
)

func main() {
	os.Exit(run())
}

func run() int {
	dir, err := worldDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakecmd:", err)
		return 125
	}
	name := filepath.Base(os.Args[0])
	args := os.Args[1:]
	cwd, _ := os.Getwd()
	inv := fakeworld.Invocation{Argv: append([]string{name}, args...), EnvNames: envNames(), Cwd: cwd}

	var rule fakeworld.Rule
	var matched bool
	err = locked(dir, func() error {
		if err := fakeworld.AppendLog(filepath.Join(dir, fakeworld.LogFile), inv); err != nil {
			return err
		}
		w, err := fakeworld.Load(filepath.Join(dir, fakeworld.WorldFile))
		if err != nil {
			return err
		}
		rule, matched = w.Match(name, args)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakecmd:", err)
		return 125
	}
	if matched && rule.SleepMS > 0 {
		time.Sleep(time.Duration(rule.SleepMS) * time.Millisecond)
	}
	res := fakeworld.Result{Stdout: rule.Stdout, Stderr: rule.Stderr, Exit: rule.Exit}
	if !matched || rule.Builtin {
		err = locked(dir, func() error {
			path := filepath.Join(dir, fakeworld.WorldFile)
			w, err := fakeworld.Load(path)
			if err != nil {
				return err
			}
			res = w.Builtin(name, args)
			if res.Mutated {
				return fakeworld.Save(path, w)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "fakecmd:", err)
			return 125
		}
	}
	fmt.Fprint(os.Stdout, res.Stdout)
	fmt.Fprint(os.Stderr, res.Stderr)
	return res.Exit
}

func worldDir() (string, error) {
	if d := os.Getenv(fakeworld.DirEnv); d != "" {
		return d, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating executable: %w", err)
	}
	return filepath.Dir(exe), nil
}

func envNames() []string {
	var names []string
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			names = append(names, kv[:i])
		}
	}
	sort.Strings(names)
	return names
}

func locked(dir string, fn func() error) error {
	f, err := os.OpenFile(filepath.Join(dir, fakeworld.LockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("opening lock: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("locking: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}
