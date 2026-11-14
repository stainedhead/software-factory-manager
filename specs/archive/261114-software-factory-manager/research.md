# Research: software-factory-manager, Increment 1

**Feature:** software-factory-manager (`facman`)
**Date:** 2026-11-14
**Source PRD:** [software-factory-manager-PRD.md](software-factory-manager-PRD.md)
**Scope:** Increment 1 (Phase 0 + Phase 1 MVP), see [spec.md](spec.md)

Most product research is already in the design doc ([§2, §2a, §3a, §3b, §4a](../../../docs/design/software-factory-installer-design.md)), checked against Homebrew and vendor docs on 2026-11-14. This file records only what Increment 1 implementation needs, and the questions still to settle while building.

## Research Questions

| # | Question | Derived from | Status |
|---|---|---|---|
| RQ-1 | What exact JSON does `brew info --json=v2 --installed` and `brew info --json=v2 --cask <id>` return for the fields we need (formula `name`, `installed[].version`, `linked_keg`, `tap`; cask `token`, `installed`, `tap`, `artifacts` with `app`, `binary`, `pkg`, `installer`)? | FR-010, FR-015, I1-2, I1-9 | Answered below (shape); fixtures are captured from real output in T-D2 |
| RQ-2 | How does a program check and request item-level tap trust in Homebrew 6 (`brew trust`, `brew trust --cask <item>`, listing what is trusted), and how is "already trusted" detected without mutating anything? | FR-047, I1-10, design §3b | Answered below; verify flags against the runner's Homebrew in I1-14 |
| RQ-3 | How is Homebrew's installer run non-interactively and safely: URL, `NONINTERACTIVE=1`, sudo needs per platform, exit codes? | FR-030, FR-051, I1-5, design §4a | Answered below |
| RQ-4 | How do we detect glibc version, WSL, display and distro from pure Go without cgo? | FR-044, FR-056 | Answered below |
| RQ-5 | How do we find an app bundle's ID and version on macOS without parsing binary plists ourselves? | FR-010 | Answered below |
| RQ-6 | What is the safest way to test a Bubble Tea TUI and a headless `[y/N]` prompt deterministically? | I1-13, Delivery plan "How it is tested" | Answered below |
| RQ-7 | Which small formula is absent on GitHub's macOS runner and safe to install in I1-14? | I1-14 | Answered: GNU `hello` |
| RQ-8 | Which provenance paths prove "OS-bundled" for git, sqlite and jq on macOS and Linux? | FR-020, Appendix E | Answered below |

## Industry Standards

- **XDG Base Directory spec**: state (not config, not cache) belongs in `$XDG_STATE_HOME`, default `~/.local/state`. Used for the Linux and WSL state dir (PRD Appendix J, review decision 22).
- **sysexits.h**: `EX_USAGE = 64` for usage errors, matching Appendix H.
- **Homebrew tap trust (Homebrew 6+)**: item-level trust is the recommended model; whole-tap trust and the `HOMEBREW_NO_REQUIRE_TAP_TRUST` opt-out are discouraged/deprecated (design §3b, docs.brew.sh/Tap-Trust).
- **Never `curl | sh`**: download to a file, then run it, so the exact bytes run are the bytes that can be inspected (FR-030).

## Existing Implementations

- **Current repo code**: `cmd/facman/main.go` uses the standard `flag` package and prints `--version`/`--about` from `internal/domain/appinfo`. Tests in `cmd/facman/main_test.go` and `internal/domain/appinfo/appinfo_test.go` must keep passing; the version is injected through `-ldflags -X main.version=...` (Makefile, GoReleaser), so `var version` stays in `package main`.
- **CI** (`.github/workflows/ci.yml`): runs `make cover`, then `go test -race -tags integration ./...`, then `make build` and `bin/facman --version` on `ubuntu-latest` and `macos-latest`. Integration tests must therefore skip cleanly on Linux, where Homebrew is at `/home/linuxbrew/.linuxbrew` but not on PATH.
- **GoReleaser** (`.goreleaser.yaml`): linux/darwin × amd64/arm64. `make build-all` mirrors this matrix.
- **Prior art**: `brew bundle` (declarative but no ownership model, overwrites), Ansible `community.general.homebrew` (no consent gate), `mise`/`asdf` (version managers, not installers). None separates `foreign` from `managed`, which is why `facman` keeps its own inventory.

## API Documentation

### RQ-1: `brew info --json=v2`

`brew info --json=v2 --installed` returns one object with two arrays:

```json
{
  "formulae": [
    { "name": "jq", "full_name": "jq", "tap": "homebrew/core",
      "installed": [ { "version": "1.7.1", "installed_on_request": true } ],
      "linked_keg": "1.7.1", "keg_only": false }
  ],
  "casks": [
    { "token": "claude-code", "full_token": "claude-code", "tap": "homebrew/cask",
      "installed": "2.1.4",
      "artifacts": [ { "binary": [ "claude" ] } ] }
  ]
}
```

- Formula installed version: last element of `installed[].version`. `keg_only: true` (sqlite, `node@24`) means nothing is linked into `{brew_prefix}/bin`.
- Cask installed version: `installed` (string or null).
- Cask artifacts (from `brew info --json=v2 --cask <id>`, which works for casks that are not installed and makes no change): `app` entries are bundle names installed into `/Applications`; `binary` entries are `[source]` or `[source, {"target": "name"}]`, linked into `{brew_prefix}/bin/<target or basename>`; `pkg` and `installer` entries imply a `.pkg`/script installer that may prompt for sudo. **Update 2026-11-14 (review FR-003, verified on the macOS CI runner, Homebrew 7.0.11):** for an *installed* cask, each artifact object carries the absolute link path in a `target` key next to its stanza key, for example `{"binary": ["bin/az"], "target": "/opt/homebrew/bin/az"}` and `{"app": ["Firefox.app"], "target": "/Applications/Firefox.app"}`. Completions (`bash_completion`, `zsh_completion`, `fish_completion`) carry a target too, and a `command_wrapper` stanza (`["firefox", {"executable": "…"}]`) writes a wrapper script at its target in `{brew_prefix}/bin`. A cask that is not installed (`brew info --json=v2 --cask zoom`) still has one-key artifacts. `facman` reads both shapes and uses the reported target as the link path when there is one (`brew.Cask.Links`).
- `brew info` on a tap item that is not tapped fails; for Orca the cask artifacts are read only after the tap step, so the conflict pre-flight for Orca uses the catalog's `detect.commands` and the bundle name until then (decision recorded in implementation-notes D-9).
- Both calls are local and read-only when `HOMEBREW_NO_AUTO_UPDATE=1` and `HOMEBREW_NO_INSTALL_FROM_API` are left alone; `facman` sets `HOMEBREW_NO_AUTO_UPDATE=1` and `HOMEBREW_NO_ENV_HINTS=1` for detection calls so they make no network calls (NFR "Detection and planning make no network calls"). Install commands run without `HOMEBREW_NO_AUTO_UPDATE`, so brew behaves as the developer expects.

Fixtures for tests are captured from real output (T-D2) and stored under `internal/adapter/brew/testdata/`.

### RQ-2: tap trust

- `brew tap stablyai/orca`: adds the tap (no trust implied).
- `brew trust --cask stablyai/orca/orca`: trusts one item. `facman` never runs `brew trust stablyai/orca` (whole tap).
- `brew trust` with no arguments lists trusted taps and items (read-only); the brew adapter parses it to decide whether the trust step is needed. `brew tap` with no arguments lists tapped taps.
- `HOMEBREW_NO_REQUIRE_TAP_TRUST` and any variable starting with that prefix are stripped from every child environment (I1-10).
- Exact output formats of `brew trust` listing are captured in T-D2 and verified in I1-14; if the format differs, the adapter treats "unknown" as "not trusted", which only adds a reviewed step (safe direction).

### RQ-3: Homebrew installer

- URL: `https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh`, downloaded with Go's `net/http` (honours `HTTPS_PROXY`), 3 attempts with exponential backoff (1 s, 2 s, 4 s), written to a `0700` temp file.
- Run as `/bin/bash <tmpfile>` with `NONINTERACTIVE=1`, as the invoking user, terminal handed to the child (it calls `sudo` itself on macOS to create `/opt/homebrew` and on Linux to create `/home/linuxbrew`).
- Exit code 0 on success. Afterwards `brew` is located at the platform prefix, not on PATH, which is why `facman` evaluates `brew shellenv`.
- No published checksum: Review marks it "unverified script" (FR-030 partial).
- The URL is a constructor parameter of `adapter/installer`, so tests point it at an `httptest` server; it is not a user flag.

### RQ-4: platform facts without cgo

| Fact | Source |
|---|---|
| OS, arch | `runtime.GOOS`, `runtime.GOARCH` |
| WSL | `WSL_DISTRO_NAME` env, or `microsoft` (case-insensitive) in `/proc/version` |
| Display | `DISPLAY` or `WAYLAND_DISPLAY` set (Linux); always true on macOS |
| glibc | `getconf GNU_LIBC_VERSION` (prints `glibc 2.39`), falling back to the first line of `ldd --version`; unknown on musl, reported as a warning |
| Distro | `/etc/os-release` `ID` and `ID_LIKE` (debian/ubuntu → apt, fedora/rhel/centos → dnf) |
| UID | `os.Getuid()` |
| Brew prefix owner | `os.Stat(prefix).Sys().(*syscall.Stat_t).Uid` compared with `os.Getuid()` |

All file paths (`/proc/version`, `/etc/os-release`) and command names are injected so tests use fixtures.

### RQ-5: app bundles

- `mdfind "kMDItemCFBundleIdentifier == '<id>'"` lists bundle paths (Spotlight; may be empty if indexing is off).
- Fallback: list `*.app` in `/Applications` and `~/Applications` (one level), and read `Contents/Info.plist` with `plutil -extract CFBundleIdentifier raw -o - <plist>` and `CFBundleShortVersionString` the same way. `plutil` ships with macOS and handles binary plists, so no plist library is needed.
- T3 Code is matched by bundle ID only, because its app name includes "(Beta)".

### RQ-8: OS-bundled tools

| Tool | macOS | Linux / WSL |
|---|---|---|
| git | `/usr/bin/git` is a CLT shim; present (OS) only if `xcode-select -p` succeeds; never executed otherwise | `/usr/bin/git` from the distro package |
| sqlite | `/usr/bin/sqlite3` | `/usr/bin/sqlite3` |
| jq | `/usr/bin/jq` (macOS 15+) | `/usr/bin/jq` |

Rule: a resolved path under `/usr/bin`, `/bin` or `/usr/sbin` gets provenance `os`. On macOS it counts as present for git, sqlite, jq (catalog `satisfies_if: { provenance: os }`); for other packages an `os` provenance is still `foreign`.

## Best Practices

- **Commands as argv, never shell strings.** No `sh -c`; every command shown in Review is exactly the `exec.Cmd` argv. Logs quote argv with `strconv.Quote` per element.
- **Context and timeouts.** Every detector and subprocess takes `context.Context`; detectors get 10 s, version probes 5 s; install commands have no timeout (they can legitimately be long) but honour cancellation only between commands (stop after current).
- **SQLite with `modernc.org/sqlite`.** Open with `_pragma=busy_timeout(5000)`, `_pragma=journal_mode(WAL)`, `_pragma=foreign_keys(1)`; one writer; migrations in a transaction; `meta.schema_version`.
- **Bubble Tea testing.** Drive `Model.Update` with messages and compare `View()` to golden files; force `lipgloss` to the ASCII profile and a fixed width/height so output is stable; long-running work is injected as `tea.Cmd` factories so tests return canned messages (no goroutines left running).
- **Headless prompt testing.** `github.com/creack/pty` gives the child a real terminal so `term.IsTerminal(stdin)` is true; tests write `y\n`, `n\n` or close the pty.
- **Fake executables over mocks.** One `fakecmd` program is copied/symlinked as `brew`, `xcode-select`, `mdfind`, `plutil`, `npm`, `getconf`, `ldd`, `file`; it answers from a JSON fixture keyed by argv, can simulate state changes (`install` adds to installed state), sleep, or fail, and appends each invocation (argv, selected env names) to a JSON-lines log. Unit tests use in-process fakes of the ports instead.
- **Determinism.** Clock, user name, PID and random IDs are injected; golden output never contains real times.

## RQ-6, RQ-7 answers

- **RQ-6:** see Best Practices (golden `View()` tests, pty-driven prompt).
- **RQ-7:** GNU `hello` (`brew install hello`): a tiny bottled formula with no dependencies, not preinstalled on GitHub's `macos-*` images, installs in seconds. The integration test checks it is absent first and skips the install part if it is already present, and uninstalls it in cleanup only if this test installed it.

## Open Questions

| # | Question | Owner | Resolution path |
|---|---|---|---|
| R-OQ-1 | Exact `brew trust` listing format on Homebrew 6 | Implementer (T-D2) | Capture on the macOS runner; unknown output = not trusted (safe) |
| R-OQ-2 | Do all "same cask" Linux entries in Appendix E really install on Linux Homebrew? | Implementer, then J's lab | Data-only fix in the catalog if not |
| R-OQ-3 | Orca bundle ID | J (I1-16 field run) | Data-only fix |
| R-OQ-4 | Bubble Tea v1 vs v2 | Implementer (T-G1) | Latest stable; ADR entry |

## References

- Design doc: [docs/design/software-factory-installer-design.md](../../../docs/design/software-factory-installer-design.md)
- Homebrew: `brew info` JSON v2 (docs.brew.sh/Querying-Brew), Tap trust (docs.brew.sh/Tap-Trust), Installation (docs.brew.sh/Installation)
- XDG Base Directory Specification (specifications.freedesktop.org)
- `modernc.org/sqlite`, `github.com/charmbracelet/bubbletea`, `github.com/spf13/cobra`, `github.com/creack/pty`
