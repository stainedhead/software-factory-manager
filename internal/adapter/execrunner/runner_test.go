package execrunner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/usecase/process"
)

const helperEnv = "EXECRUNNER_HELPER"

// TestHelperProcess is the child program for these tests, selected by
// EXECRUNNER_HELPER. It is not a real test.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		t.Skip("helper process only")
	}
	args := os.Args[slices.Index(os.Args, "--")+1:]
	switch mode {
	case "echo":
		_, _ = fmt.Fprintln(os.Stdout, strings.Join(args, "|"))
		_, _ = fmt.Fprintln(os.Stderr, "to stderr")
		os.Exit(3)
	case "env":
		for _, kv := range os.Environ() {
			fmt.Println(kv)
		}
	case "sleep":
		time.Sleep(10 * time.Second)
	case "cat":
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		_, _ = fmt.Fprint(os.Stdout, "got "+line)
	}
	os.Exit(0)
}

func helperCmd(mode string, args ...string) process.Cmd {
	return process.Cmd{
		Argv: append([]string{os.Args[0], "-test.run=TestHelperProcess", "--"}, args...),
		Env:  map[string]string{helperEnv: mode},
	}
}

func newRunner(extra ...string) *Runner {
	return New(Options{BaseEnv: append([]string{"PATH=" + os.Getenv("PATH"), "HOME=/home/test"}, extra...)})
}

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	var log bytes.Buffer
	cmd := helperCmd("echo", "a b", "$HOME; echo hi")
	cmd.Env["SECRET_TOKEN"] = "s3cr3t"
	cmd.Log = &log
	res, err := newRunner().Run(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 || res.Success() {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "a b|$HOME; echo hi" {
		t.Errorf("stdout = %q (arguments must reach the child literally, no shell)", res.Stdout)
	}
	if !strings.HasPrefix(res.Stderr, "to stderr\n") {
		t.Errorf("stderr = %q", res.Stderr)
	}
	logText := log.String()
	for _, want := range []string{`$ "` + os.Args[0] + `"`, "# env: " + helperEnv + " SECRET_TOKEN\n", "a b|$HOME", "to stderr", "# exit 3"} {
		if !strings.Contains(logText, want) {
			t.Errorf("log %q does not contain %q", logText, want)
		}
	}
	if strings.Contains(logText, "s3cr3t") {
		t.Error("log contains an environment value")
	}
}

func TestEnvironment(t *testing.T) {
	r := newRunner("HOMEBREW_NO_REQUIRE_TAP_TRUST=1", "KEEP=base", "OVERRIDE=base")
	cmd := helperCmd("env")
	cmd.Env["OVERRIDE"] = "new"
	cmd.Env["HOMEBREW_NO_REQUIRE_TAP_TRUST_FOR_CASKS"] = "1"
	var log bytes.Buffer
	cmd.Log = &log
	res, err := r.Run(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	env := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	for _, want := range []string{"KEEP=base", "OVERRIDE=new", "HOME=/home/test"} {
		if !slices.Contains(env, want) {
			t.Errorf("child env lacks %s", want)
		}
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "HOMEBREW_NO_REQUIRE_TAP_TRUST") {
			t.Errorf("child env contains %s", kv)
		}
	}
	if strings.Contains(log.String(), "HOMEBREW_NO_REQUIRE_TAP_TRUST") {
		t.Error("log header names a stripped variable")
	}
	if got := r.Environ(nil); slices.ContainsFunc(got, func(kv string) bool { return strings.HasPrefix(kv, "HOMEBREW_NO_REQUIRE") }) {
		t.Errorf("Environ = %v", got)
	}
	if !slices.IsSorted(r.Environ(map[string]string{"A": "1"})) {
		t.Error("Environ is not sorted")
	}
}

func TestTimeoutKillsChild(t *testing.T) {
	cmd := helperCmd("sleep")
	cmd.Timeout = 200 * time.Millisecond
	start := time.Now()
	res, err := newRunner().Run(context.Background(), cmd)
	if !errors.Is(err, process.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if res.ExitCode != -1 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s; the child was not killed", time.Since(start))
	}
}

func TestCancelKillsChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := newRunner().Run(ctx, helperCmd("sleep"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestInteractiveUsesTerminal(t *testing.T) {
	var out, errOut, log bytes.Buffer
	r := New(Options{BaseEnv: []string{"PATH=" + os.Getenv("PATH")}, Stdin: strings.NewReader("password\n"), Stdout: &out, Stderr: &errOut})
	cmd := helperCmd("cat")
	cmd.Interactive = true
	cmd.Log = &log
	res, err := r.Run(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "got password\n" {
		t.Errorf("terminal stdout = %q", out.String())
	}
	if res.Stdout != "" || strings.Contains(log.String(), "password") {
		t.Errorf("interactive output was captured: %q / %q", res.Stdout, log.String())
	}
	if !strings.Contains(log.String(), "# interactive") {
		t.Errorf("log = %q", log.String())
	}
}

func TestPathLookup(t *testing.T) {
	dir := t.TempDir()
	prog := filepath.Join(dir, "fakeprog")
	if err := os.Link(os.Args[0], prog); err != nil {
		b, rerr := os.ReadFile(os.Args[0])
		if rerr != nil {
			t.Fatal(rerr)
		}
		if err := os.WriteFile(prog, b, 0o755); err != nil { //nolint:gosec // test executable
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notexec"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := New(Options{BaseEnv: []string{"PATH=" + dir}})
	if p, err := r.LookPath("fakeprog"); err != nil || p != prog {
		t.Errorf("LookPath = %q, %v", p, err)
	}
	if p, err := r.LookPath(prog); err != nil || p != prog {
		t.Errorf("LookPath(abs) = %q, %v", p, err)
	}
	for _, name := range []string{"notexec", "missing", filepath.Join(dir, "missing")} {
		if _, err := r.LookPath(name); !errors.Is(err, process.ErrNotFound) {
			t.Errorf("LookPath(%s) err = %v", name, err)
		}
	}
	// the command's PATH override decides where argv[0] is found
	cmd := process.Cmd{Argv: []string{"fakeprog", "-test.run=TestHelperProcess", "--", "x"}, Env: map[string]string{helperEnv: "echo", "PATH": dir}}
	res, err := New(Options{BaseEnv: []string{"PATH=/nonexistent"}}).Run(context.Background(), cmd)
	if err != nil || strings.TrimSpace(res.Stdout) != "x" {
		t.Errorf("run via PATH override: %+v, %v", res, err)
	}
	if _, err := r.Run(context.Background(), process.Cmd{Argv: []string{"missing"}}); !errors.Is(err, process.ErrNotFound) {
		t.Errorf("missing program err = %v", err)
	}
	if _, err := r.Run(context.Background(), process.Cmd{}); !errors.Is(err, process.ErrNotFound) {
		t.Errorf("empty argv err = %v", err)
	}
	if _, err := lookPath("x", string(os.PathListSeparator)+dir); err == nil {
		t.Error("x should not be found")
	}
}

type failWriter struct{ after int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("disk full")
	}
	f.after--
	return len(p), nil
}

func TestLogWriteFailure(t *testing.T) {
	cmd := helperCmd("env")
	cmd.Log = &failWriter{}
	if _, err := newRunner().Run(context.Background(), cmd); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("header write failure: %v", err)
	}
}

func TestStartFailure(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad")
	if err := os.WriteFile(bad, []byte("not a program"), 0o755); err != nil { //nolint:gosec // test file
		t.Fatal(err)
	}
	_, err := newRunner().Run(context.Background(), process.Cmd{Argv: []string{bad}})
	if err == nil {
		t.Error("running a non-executable file should fail")
	}
}

func TestSystemClockAndUser(t *testing.T) {
	if d := time.Since(SystemClock{}.Now()); d < 0 || d > time.Minute {
		t.Errorf("clock off by %s", d)
	}
	tests := []struct {
		name    string
		current func() (*user.User, error)
		env     string
		want    string
	}{
		{"os user", func() (*user.User, error) { return &user.User{Username: "j"}, nil }, "x", "j"},
		{"fallback to USER", func() (*user.User, error) { return nil, errors.New("no passwd") }, "dev", "dev"},
		{"unknown", func() (*user.User, error) { return nil, errors.New("no passwd") }, "", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := SystemUser{Current: tt.current, Getenv: func(string) string { return tt.env }}
			if got := u.Username(); got != tt.want {
				t.Errorf("Username = %q, want %q", got, tt.want)
			}
		})
	}
	if (SystemUser{}).Username() == "" {
		t.Error("default user is empty")
	}
	if New(Options{}).stdin != os.Stdin {
		t.Error("default stdin is not os.Stdin")
	}
}
