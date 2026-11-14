# internal/adapter/installer

Downloads the official Homebrew installer for the `bootstrap-homebrew` action (FR-030, FR-051) and implements `execute.HomebrewInstaller`.

| File | Contents |
|---|---|
| `installer.go` | `New(Options{URL, Client, TempDir, Sleep})`, `Download(ctx)` (GET to a new `0600` temp file, 3 attempts with a growing backoff, a cancel stops at once, no file left on failure), `Command(path)` = `/bin/bash <path>` (`NeedsSudo`, `Interactive`, `Unverified`), `Cleanup()` (removes the downloaded files) |
| `installer_test.go` | `httptest` server: the script lands in a file and is run from it, never piped; retries and backoff; failure names the URL and leaves nothing; cancel |

- **Never piped.** The script is saved to a file and `bash` reads that file, so the bytes that run are the bytes on disk and the command shown in Review (`/bin/bash "<temp file>"`) is the command run. `execute` checks the program matches the consented one before running it.
- **Unverified.** Homebrew publishes no checksum for `install.sh`, so the command is marked unverified in Review.
- **No `NONINTERACTIVE=1`.** facman hands the terminal to the installer, which may need the user's sudo password; with `NONINTERACTIVE` set the installer refuses to ask (implementation-notes I-32). The installer turns non-interactive by itself when stdin is not a terminal.

**Architecture.** Adapter; imports `domain/plan` and `usecase/execute` (port assertion). Wired in [cmd/facman](../../../cmd/facman/). Back to [internal/adapter](../README.md).
