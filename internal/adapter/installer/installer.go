// Package installer downloads the official Homebrew installer to a temp
// file and builds the command that runs it (FR-030, FR-051). The script is
// never piped to a shell: it is saved to a file that bash reads, so the
// exact bytes run are the bytes on disk.
package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/stainedhead/software-factory-manager/internal/domain/plan"
	"github.com/stainedhead/software-factory-manager/internal/usecase/execute"
)

// DefaultURL is the official Homebrew installer.
const DefaultURL = "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"

// Attempts is how many times Download tries before it gives up.
const Attempts = 3

// Shell is the program that runs the downloaded script.
const Shell = "/bin/bash"

// maxScript bounds the download (the real script is about 40 KB).
const maxScript = 4 << 20

// Options configure an Installer. Zero values take the real defaults.
type Options struct {
	// URL is the installer; default DefaultURL.
	URL string
	// Client fetches it; default an http.Client with a 60 s timeout.
	Client *http.Client
	// TempDir holds the downloaded file; default os.TempDir().
	TempDir string
	// Sleep waits between attempts; default a timer that honours ctx.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Installer implements execute.HomebrewInstaller.
type Installer struct {
	opts  Options
	mu    sync.Mutex
	files []string
}

var _ execute.HomebrewInstaller = (*Installer)(nil)

// New returns an Installer.
func New(opts Options) *Installer {
	if opts.URL == "" {
		opts.URL = DefaultURL
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 60 * time.Second}
	}
	if opts.Sleep == nil {
		opts.Sleep = sleepCtx
	}
	return &Installer{opts: opts}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("waiting to retry: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}

// Command returns the command that runs the script at path: /bin/bash
// <path>. Review shows it with the placeholder path; execute runs it on
// the downloaded file. It needs the terminal (the installer may ask for
// sudo) and is unverified (Homebrew publishes no checksum).
func (*Installer) Command(path string) plan.Command {
	return plan.Command{Argv: []string{Shell, path}, NeedsSudo: true, Interactive: true, Unverified: true, Mutating: true}
}

// Download fetches the installer into a new temp file (0600) and returns
// its path. It tries Attempts times with a growing backoff; a cancel stops
// it at once. On failure no file is left behind.
func (in *Installer) Download(ctx context.Context) (string, error) {
	var last error
	for i := range Attempts {
		if i > 0 {
			if err := in.opts.Sleep(ctx, time.Duration(i)*2*time.Second); err != nil {
				return "", fmt.Errorf("downloading %s: %w", in.opts.URL, err)
			}
		}
		path, err := in.fetch(ctx)
		if err == nil {
			return path, nil
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf("downloading %s: %w", in.opts.URL, ctx.Err())
		}
		last = err
	}
	return "", fmt.Errorf("downloading %s failed %d times (install Homebrew by hand from https://brew.sh, then run facman again): %w", in.opts.URL, Attempts, last)
}

func (in *Installer) fetch(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.opts.URL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("building the request: %w", err)
	}
	resp, err := in.opts.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching: HTTP %s", resp.Status)
	}
	f, err := os.CreateTemp(in.opts.TempDir, "facman-homebrew-install-*.sh")
	if err != nil {
		return "", fmt.Errorf("creating the temp file: %w", err)
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxScript))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n == 0 {
		err = errors.New("the server sent an empty script")
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("saving the script: %w", err)
	}
	in.mu.Lock()
	in.files = append(in.files, f.Name())
	in.mu.Unlock()
	return f.Name(), nil
}

// Cleanup removes every file Download saved.
func (in *Installer) Cleanup() {
	in.mu.Lock()
	defer in.mu.Unlock()
	for _, f := range in.files {
		_ = os.Remove(f)
	}
	in.files = nil
}
