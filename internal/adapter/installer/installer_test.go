package installer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
)

var _ execute.HomebrewInstaller = (*Installer)(nil)

const script = "#!/bin/bash\necho installing homebrew\n"

func newInstaller(t *testing.T, url string) (*Installer, *[]time.Duration) {
	t.Helper()
	var sleeps []time.Duration
	in := New(Options{URL: url, TempDir: t.TempDir(), Sleep: func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return ctx.Err()
	}})
	return in, &sleeps
}

func TestDownloadWritesTempFileNeverPiped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(script)) }))
	defer srv.Close()
	in, sleeps := newInstaller(t, srv.URL)
	path, err := in.Download(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != script {
		t.Fatalf("file %s = %q, %v", path, b, err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 (read by bash, never executed directly)", fi.Mode().Perm())
	}
	cmd := in.Command(path)
	if got := strings.Join(cmd.Argv, " "); got != "/bin/bash "+path {
		t.Errorf("argv = %q: the script must be run from the file, never piped to a shell", got)
	}
	if !cmd.NeedsSudo || !cmd.Interactive || !cmd.Unverified || !cmd.Mutating {
		t.Errorf("command flags = %+v", cmd)
	}
	if len(*sleeps) != 0 {
		t.Errorf("slept %v on success", *sleeps)
	}
	in.Cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Cleanup left %s", path)
	}
}

func TestDownloadRetriesThreeTimes(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(script))
	}))
	defer srv.Close()
	in, sleeps := newInstaller(t, srv.URL)
	if _, err := in.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || len(*sleeps) != 2 || (*sleeps)[1] <= (*sleeps)[0] {
		t.Errorf("calls = %d, sleeps = %v; want 3 calls with growing backoff", calls.Load(), *sleeps)
	}
}

func TestDownloadFailsAfterThreeAttempts(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"server error", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "down", http.StatusBadGateway) }},
		{"empty body", func(http.ResponseWriter, *http.Request) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); tt.handler(w, r) }))
			defer srv.Close()
			in, _ := newInstaller(t, srv.URL)
			_, err := in.Download(context.Background())
			if err == nil || !strings.Contains(err.Error(), srv.URL) {
				t.Fatalf("err = %v, want it to name the URL", err)
			}
			if calls.Load() != Attempts {
				t.Errorf("calls = %d, want %d", calls.Load(), Attempts)
			}
			entries, _ := os.ReadDir(in.opts.TempDir)
			if len(entries) != 0 {
				t.Errorf("temp files left: %v", entries)
			}
		})
	}
}

func TestDownloadCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "x", 500) }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	in := New(Options{URL: srv.URL, TempDir: t.TempDir(), Sleep: func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}})
	_, err := in.Download(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	entries, _ := os.ReadDir(in.opts.TempDir)
	if len(entries) != 0 {
		t.Errorf("temp files left: %v", entries)
	}
}

func TestDefaults(t *testing.T) {
	in := New(Options{})
	if in.opts.URL != DefaultURL || in.opts.Client == nil || in.opts.Sleep == nil {
		t.Errorf("defaults = %+v", in.opts)
	}
	if got := in.Command("<temp file>").String(); got != `/bin/bash "<temp file>"` {
		t.Errorf("review command = %q", got)
	}
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Error(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); err == nil {
		t.Error("sleepCtx ignored the cancel")
	}
	bad := New(Options{URL: "http://127.0.0.1:1/", TempDir: filepath.Join(t.TempDir(), "missing"), Sleep: func(context.Context, time.Duration) error { return nil }})
	if _, err := bad.Download(context.Background()); err == nil {
		t.Error("want an error")
	}
	if _, err := New(Options{URL: "://bad", Sleep: func(context.Context, time.Duration) error { return nil }}).Download(context.Background()); err == nil {
		t.Error("want an error for a bad URL")
	}
}
