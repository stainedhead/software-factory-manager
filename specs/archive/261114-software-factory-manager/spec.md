# Spec: software-factory-manager, Increment 1 (Phase 0 + Phase 1 MVP)

**Feature:** software-factory-manager (`facman`, the Agentic Software Factory Manager)
**Spec directory:** `specs/261114-software-factory-manager/`
**Created:** 2026-11-14
**Source PRD:** [software-factory-manager-PRD.md](software-factory-manager-PRD.md) (reviewed 2026-11-14, Appendix L)
**Status:** Reviewed (`/review-spec`, 2026-11-14): implementation-ready. See [Spec review log](#spec-review-log-2026-11-14).

> **Scope of this spec: Increment 1 only.** The PRD describes the whole product and splits delivery into increments (PRD, *Delivery plan*). This spec covers **Increment 1 = Phase 0 + Phase 1 (MVP)** and nothing else. Every requirement the Delivery plan does not put in Increment 1 is listed under [Deferred requirements](#deferred-requirements-out-of-scope-for-increment-1) and gets its own spec later. Where this spec and the PRD's full-product text differ, the Delivery plan's Increment 1 scope and the decisions in this spec win for this increment.

## Executive Summary

Increment 1 delivers the smallest slice of `facman` that is useful on its own: on a macOS or Linux machine, a developer runs `facman` (TUI) or `facman apply --profile agentic-factory` (headless). `facman` checks the machine's bootstrap state, detects every catalog v1 package and how it got there, classifies each one into an ownership state, and proposes only the missing brew-installable tools, with the exact commands. After explicit consent it installs them through the `brew-formula` and `brew-cask` backends, recording every step in a local SQLite inventory with an append-only audit log. It never touches what it did not install, never overwrites a conflicting file, and trusts Orca's third-party tap item by item. `scan`, `plan`, `status` and `doctor` give read-only views with `--json` output (report schema v1).

## Problem Statement

Lab machines arrive in every state: tools installed through Homebrew, by vendor scripts, bundled with the OS, or installed twice. Hand-run install docs drift and blunt scripts overwrite what developers care about. Increment 1 addresses the core of that problem for developers on lab machines (mostly single-user Macs) and for Linux machines with Homebrew: **detect first, propose only what is missing, ask before acting, manage only what was installed**. Unattended configuration lists for build machines (Increment 3), lifecycle management (Increment 2) and tool configuration seeding (Increment 5+) build on this slice.

## Goals / Non-Goals

### Goals (Increment 1)

- **G1 (partial).** A supported machine reaches the brew-installable part of the declared toolset (catalog v1) through one tool.
- **G2.** Safe by default: no change without consent, every change shown and recorded, foreign tools and conflicting files never touched, item-level tap trust only.
- **Phase 0.** `facman scan --json` produces a provenance report that J can run on 3 to 5 lab Macs to validate the detection rules (I1-16, manual, after shipping).
- A codebase that follows AGENTS.md (Clean Architecture, TDD, docs current) and that later increments extend without restructuring.

### Non-Goals (Increment 1)

- Everything listed under [Deferred requirements](#deferred-requirements-out-of-scope-for-increment-1), notably: `upgrade`, `uninstall`, `adopt`, `forget`, `restore`, move-aside, the npm backend, Paperclip and its service, `-nop`, configuration lists (`-cl`), setup flags, `template`/`validate`, `export`, the short-flag normalizer, signed catalog pulls and signed releases.
- The product-level non-goals in the PRD all still apply (not an MDM, no central server, no native Windows backend, no secrets in files, no multi-user brew ownership, no Gemini CLI).
- Writing to the developer's login profile. Summary prints the lines to add instead.
- Installing Linux distro prerequisites. They are reported with the exact command, and `apply` exits 4.

## User Requirements

### User stories

- **US-1 (developer, TUI).** As a developer on a lab Mac, I run `facman`, see what I already have and how it was installed, tick what I want, review the exact commands, consent once, and get the missing tools without losing my own setup.
- **US-2 (developer, headless).** As a developer or operator, I run `facman plan --profile agentic-factory` to see the commands, then `facman apply --profile agentic-factory`, answer `y` at a `[y/N]` prompt, and get the same result as the TUI.
- **US-3 (scripts and support).** As someone diagnosing a machine, I run `facman scan --json`, `facman status --json` and `facman doctor --json` and get machine-readable, deterministic output and exit codes.
- **US-4 (factory owner).** As a factory owner, I point `facman` at a checked-in catalog directory with `--catalog ./catalog` to try catalog changes before they ship.

### Functional Requirements

Requirement numbers are the PRD's, so they trace one to one. "Full" means the whole PRD requirement is in Increment 1. "Partial" means only the stated part is, and the rest is deferred. Clarifications decided in this spec are marked **[spec]** and logged in [implementation-notes.md](implementation-notes.md#technical-decisions).

#### Identity and CLI

| FR | Scope | Increment 1 requirement |
|---|---|---|
| FR-001 | Full | Built and invoked as `facman` (entry point `cmd/facman`). The full name "Agentic Software Factory Manager" appears in text output and comes only from `internal/domain/appinfo`. |
| FR-002 | Full | `facman --version` prints one line with the full name. `facman --about` prints title, description, version and repository. Existing behaviour and tests are kept. |
| FR-003 | Full | Single static Go executable: `make build` uses `CGO_ENABLED=0`. `make build-all` cross-compiles darwin and linux for amd64 and arm64 into `bin/<os>-<arch>/facman`, matching `.goreleaser.yaml`. |
| FR-004 | Partial | Commands `scan`, `plan`, `apply --profile`, `status`, `doctor`. `facman` with no command opens the TUI. Any other command name (including `upgrade`, `uninstall`, `adopt`, `forget`, `restore`, `setup`, `service`, `export`, `config`, `template`, `validate`) is a usage error, exit 64, with a hint that it arrives in a later increment. |
| FR-006 | Full | A value flag with no value (for example `--profile` with nothing after it) is exit 64. No implicit default file or profile in headless commands. |
| FR-007 | Partial | `--json` on `scan`, `plan`, `apply`, `status`, `doctor`, using **report schema v1** (defined in this increment, embedded in the binary, reused by `export` later). |

#### Detection and ownership

| FR | Scope | Increment 1 requirement |
|---|---|---|
| FR-010 | Full | Detection runs first and independently of backends: PATH (`exec.LookPath` then symlink resolution for provenance), one `brew info --json=v2 --installed` call, app bundles by bundle ID (`mdfind`, with a fallback scan of `/Applications` and `~/Applications`), and `npm ls -g --json` when npm is present. Detectors run in parallel, each with a 10 s timeout; a timed-out detector reports `unknown` with the reason. |
| FR-011 | Full | Each package is in exactly one state: `absent`, `foreign`, `managed`, `adopted`, `drifted`, `duplicate`, `conflict`, plus `unavailable-here` (FR-044). `adopted` can only come from an existing inventory row in this increment, because the Adopt action is deferred. |
| FR-012 | Partial | Enforced invariant: `facman` never issues a command against a `foreign`, `duplicate`, `drifted`, `managed` or `adopted` package in this increment (install only, so only `absent` packages get commands). `foreign` rows are shown, not selectable. Adopt is deferred. |
| FR-013 | Partial | `drifted` is detected and flagged. Reinstall and Forget actions are deferred. |
| FR-014 | Full | `duplicate` packages are reported with the install that wins on PATH. Never resolved. |
| FR-015 | Full | Conflict pre-flight before planning. Paths checked: for casks, the `app` and `binary` artifacts from `brew info --json=v2 --cask <id>`; for formulae, each `detect.commands` name under `{brew_prefix}/bin`. A path holding a file or symlink not owned by this package marks it `conflict`, with owner class: this package, another brew item, npm, another installer, or orphan. |
| FR-016 | Full | Conflict evidence: `ls -l`-style line, file type, symlink target and whether it dangles, executable bit, size, modification time, the owner found, and the command that would fail. Shown on `i` in the TUI and in `--json`. |
| FR-017 | Partial | **Detection and refusal only.** A `conflict` package is not selectable and gets no command. Evidence and a manual-fix hint are shown. `--force` and `--overwrite` are never passed to brew (and no `brew` invocation contains `--force` at all, see D-3). Move-aside and `restore` are deferred. |
| FR-018 | Full | `facman doctor [--json] [--catalog <dir>]` runs the conflict scan and the trust check of the catalog's own taps for the whole catalog without planning. Exit 0 when clean or info-only, 2 on a conflict or an untrusted tap that a catalog item needs. |
| FR-020 | Full | `/usr/bin/git` is never executed before `xcode-select -p` succeeds. OS-bundled git, sqlite and jq show as `foreign` with provenance `os` ("present (OS)"). |

#### Planning, consent and execution

| FR | Scope | Increment 1 requirement |
|---|---|---|
| FR-025 | Full | Every mutation is a list of concrete commands (argv, never shell strings), shown before consent; the same list is printed by `--dry-run` and `plan`. |
| FR-026 | Partial | TUI flow Scan → Plan → Review → Run → Summary, **install only**. Keys: `space`, `i`, `/`, `Enter`, `Esc`, `↑`/`↓`/`k`/`j`, `?`, `q`, `Ctrl+C` (PRD Appendix C). Not yet: `a`, `r`, `s`, Manage and Files tabs (`Tab` shows "available in a later increment"). The root command accepts `--profile` (repeatable, default `agentic-factory`), `--with` and `--catalog` with the same rules as `plan` **[review]**. |
| FR-027 | Full | Consent is recorded on every `operations` row as `consent_by = <os user>@<RFC 3339 UTC time>` for both the TUI and headless `apply --profile`. |
| FR-028 | Full | `requires:` dependencies are ordered first (stable topological order; ties broken by catalog order). A requires cycle is a catalog validation error (exit 3). |
| FR-029 | Partial | `Backend` port with `brew-formula` and `brew-cask` adapters, plus the `report` and `host-report` pseudo-backends (never mutate, produce `unavailable-here` with hint). |
| FR-030 | Partial | Applies to the Homebrew installer only: downloaded to a temp file (never piped), run with `/bin/bash <file>`, marked "unverified script" in Review (Homebrew publishes no SHA-256). |
| FR-031 | Full | Homebrew is never run as root (UID 0 → exit 4). Steps needing sudo (cask `pkg`/`installer` artifacts, the Homebrew installer) hand the terminal to the child process; `facman` never reads a password. |
| FR-032 | Full | Each action is written to `operations` as `in_progress` before it runs and updated with exit code, status and log path after. On the next launch, leftover `in_progress` rows are marked `interrupted` and their packages re-detected; the next `apply` finishes the remaining work. |

#### Catalog, profiles and platforms

| FR | Scope | Increment 1 requirement |
|---|---|---|
| FR-040 | Full | Catalog v1 (PRD Appendix E) is embedded (`embed`) as YAML: all 25 Appendix E rows (including the `container-runtime` choice group), plus the choice members `orbstack`, `colima`, `docker-desktop` and colima's helper formulae `docker` and `docker-compose` as catalog entries. |
| FR-041 | Full | Codex CLI, uv, jq and ripgrep are on by default; optional items are off by default. |
| FR-042 | Partial | YAML loading, `include:`, `optional`, choice groups; validated in Go at load (unknown IDs, duplicates, include cycles, requires cycles, conflicting pins). Publishing the JSON Schema is deferred (Increment 4). |
| FR-043 | Full | Every entry declares `kind` and per-platform `install` blocks (`darwin`, `linux`, `wsl`). Missing block = not offered here. On WSL, `cli` entries inherit `linux`; `gui` entries default to `host-report`. |
| FR-044 | Partial | Platform detection: OS, arch, WSL, display, glibc. Wanted items with no implemented backend on this platform are `unavailable-here` with reason and hint (e.g. Paperclip: "needs the npm backend (Increment 2)"); on Linux without a display, `gui` items are `unavailable-here`. Never blocks the run. |
| FR-045 | Partial | On WSL, GUI items are `unavailable-here` with the catalog's `winget` hint. Host detection is deferred. |
| FR-047 | Full | Orca: `brew tap stablyai/orca` and `brew trust --cask stablyai/orca/orca` are separate reviewed steps before its install, each omitted only when already in place; grants recorded in `tap_trust`. Whole-tap trust is never requested; `HOMEBREW_NO_REQUIRE_TAP_TRUST*` is removed from every child environment. |
| FR-079 | Partial | `plan` and `apply` take `--profile <name>` (required, repeatable, union: required beats optional, conflicting version pins exit 3 naming both profiles), `--with <id>` (repeatable; for a choice group, `--with <member-id>` picks that member) and `--catalog <dir>`. `--set` and git-URL catalogs are usage errors (exit 64) in this increment. |

#### Startup bootstrap

| FR | Scope | Increment 1 requirement |
|---|---|---|
| FR-050 | Full | Every command except `--version`, `--about` and `--help` runs the bootstrap check first: platform, Xcode CLT (macOS), Linux prerequisites, Homebrew (PATH, then `/opt/homebrew`, `/usr/local`, `/home/linuxbrew/.linuxbrew`), brew prefix ownership, shell environment, Node/npm. |
| FR-051 | Partial | Missing Homebrew: `scan`, `plan`, `status`, `doctor` report it and never install. In the TUI and `apply --profile`, the official installer (`NONINTERACTIVE=1`, invoking user, temp file) is the first step in Review behind the same consent. |
| FR-052 | Partial | Node/npm is checked and reported (version, provenance, ≥ 24.11). `node@24` is installed only as the `base-dev` `node` formula. |
| FR-053 | Partial | `brew shellenv` is evaluated for `facman` and its children; `node@24`'s keg `bin` is put on the children's PATH. Summary prints the exact profile lines to add. Writing the profile is deferred. |
| FR-055 | Full | Homebrew installed by `facman` is recorded `managed` with `bootstrap: true` (package id `homebrew`). A pre-existing Homebrew is `foreign` and used. Nothing uninstalls Homebrew. |
| FR-056 | Partial | UID 0 → exit 4 with a fix. Homebrew found at known prefixes when not on PATH. glibc < 2.39 warns (always in `scan`/`plan`, and in `--dry-run`). Distro from `/etc/os-release`. Missing Linux prerequisites are reported with the exact apt or dnf command, and `apply` stops with exit 4; an unknown distro with missing prerequisites is exit 4 on `apply`. |

#### Inventory

| FR | Scope | Increment 1 requirement |
|---|---|---|
| FR-060 | Full | Embedded SQLite (`modernc.org/sqlite`, no cgo) at the Appendix J path. Migration 1 creates `packages`, `operations`, `detections`, `services` (empty), `meta`, `tap_trust`. `meta.schema_version` tracks migrations. |
| FR-061 | Full | Every launch reconciles inventory with detection; disagreements show as `drifted`. |
| FR-062 | Full | The DB is inspectable with the system `sqlite3`; `facman` never depends on the catalog `sqlite` package. |

#### Execution flags, exit codes, Summary

| FR | Scope | Increment 1 requirement |
|---|---|---|
| FR-074 | Partial | `--dry-run` on `apply` prints the plan and changes nothing (no `packages` or `operations` rows, no mutating subprocess). `--check` is deferred. |
| FR-075 | Partial | Exit codes 0, 1, 3, 4, 64, and 2 from `doctor`. Precedence rules in [Edge cases](#edge-cases-and-error-handling). |
| FR-076 | Partial | Behaviour for `apply --profile`: a second run proposes and runs nothing and exits 0. |
| FR-112 | Partial | Summary shows "installed, sign-in required" next steps for Claude Code, Claude desktop, Copilot CLI, GitHub Copilot app and Antigravity CLI. Policy checks deferred. |

#### Increment 1 behaviour decisions (from the PRD Delivery plan)

- **Headless consent.** `apply --profile` prints the full command list, then asks `Proceed? [y/N]` on the terminal (prompt on stderr). Default No. Non-terminal stdin → exit 64 pointing to the TUI (and to `-cl` in Increment 3). No `--yes`. Declining (including `n`, empty line, EOF and `Ctrl+C` at the prompt **[spec]**) changes nothing, writes no `operations` rows, exits 0.
- **Profiles.** `plan` and `apply` require `--profile` (absent → exit 64). The TUI defaults to `agentic-factory` and shows it in the header.
- **Conflicts in a run.** A selected package in `conflict` is left out with everything that `requires:` it; the rest proceeds. Summary and `--json` list it as "not installed: conflict"; exit 1.
- **Sudo.** TUI: `tea.ExecProcess` hands the terminal to the child. Headless: the child inherits the terminal. `facman` never reads a password.

### Deferred requirements (out of scope for Increment 1)

| Deferred to | FRs |
|---|---|
| Increment 2 (Manage) | Actions of FR-012/FR-013 (Adopt, Reinstall, Forget), FR-017 move-aside and `restore`, FR-019 version policy, FR-029 `npm` backend, FR-033, FR-080 to FR-087 (Paperclip service, exposure, backup and rollback, `-nop`), the rest of FR-004 (headless parity), Manage tab, `a`/`r` keys |
| Increment 3 (Configuration lists and Linux CI) | FR-005 short-flag normalizer, FR-054, the rest of FR-056 (installing distro prerequisites), FR-070 to FR-079c except the parts of FR-076/FR-079 above (`-cl`, `-c`, `--check`, `--set`, `config init/export/validate`, `prune`, `s` key), the `-cl` part of FR-051 |
| Increment 4 (Formats) | FR-100 to FR-104, the published JSON Schema part of FR-042, Files tab |
| Increment 5 onwards | FR-046 (signed catalog pulls, git-URL catalogs, `min_facman_version`), FR-048 (untrusted foreign tap reports), FR-090 to FR-095 (setup flags), FR-110 (`export`), FR-111 (proxy CA, mirrors; Go's default `HTTPS_PROXY` handling still applies), FR-113 (signing, notarization, Homebrew tap, self-update), host GUI detection (rest of FR-044/FR-045), the rest of FR-052 (npm-driven Node install) and FR-053 (profile write) |

## Non-Functional Requirements

| Area | Increment 1 requirement | Verified by |
|---|---|---|
| Performance | `--version` < 100 ms; first TUI frame < 500 ms; bootstrap check with brew present < 2 s; `scan` of the full catalog < 5 s p95 (parallel detectors, 10 s per-detector timeout → `unknown`). Detection and planning make no network calls. | I1-2 (scan bound in e2e), perf smoke tests in T-F6; reference-hardware numbers recorded manually in implementation-notes |
| Reliability | Idempotent, resumable runs (FR-032, FR-076). Homebrew installer download retries 3 times with backoff; package installs are never retried. A failed install skips its dependents, the run continues, exit 1. Zero flakes caused by `facman`: tests use fakes, fixed clocks and no sleeps on real time. | I1-7, I1-11, unit tests |
| Security | No root brew, no captured credentials, no `curl \| sh`, no shell strings (argv only), no `--force`/`--overwrite`, item-level trust only, `HOMEBREW_NO_REQUIRE_TAP_TRUST*` stripped from children, commands logged with env var names not values. | I1-5, I1-8, I1-9, I1-10 |
| Observability | Append-only `operations` audit log; per-operation log file `<state dir>/logs/<YYYYMMDD-HHMMSS>-<operation id>-<package id>.log` recorded in `operations.log_path`; logs older than 90 days pruned at launch (rows kept; a pruned log shows "pruned"); `--json` on every Increment 1 command; deterministic exit codes. | I1-7, unit tests |
| Portability | macOS (primary) and Linux (glibc ≥ 2.39 for bottles, x86_64 and ARM64); WSL reports GUI items as host items. `CGO_ENABLED=0`. State under the XDG state dir on Linux/WSL. | I1-1, platform unit tests |
| Usability | Exact commands and evidence before any change; errors name the cause and the fix. | I1-12, I1-13 |
| Maintainability | Clean Architecture, TDD, ≥ 80% coverage with new code fully covered, READMEs in every folder, docs updated in the same change. | I1-15 |

## System Architecture

Summary only; full design in [architecture.md](architecture.md).

- **Domain (`internal/domain/...`)**: `appinfo` (exists), `version`, `platform`, `catalog`, `ownership`, `plan`, `inventory`, `failure`. Pure Go, no I/O.
- **Use case (`internal/usecase/...`)**: `bootstrap`, `scan` (detect + reconcile + classify), `conflicts`, `planning`, `execute`, `doctor`, `status`, `summary`. Each declares the ports it consumes.
- **Adapters (`internal/adapter/...`)**: `execrunner`, `platform`, `statedir`, `sqlite`, `catalogyaml` (embedded catalog v1 + `--catalog` dir), `brew`, `detect`, `fsprobe`, `installer`, `report`, `cli` (cobra), `tui` (Bubble Tea).
- **Entry point (`cmd/facman`)**: wiring only.

## Scope of Changes

### Files and folders to create

| Path | Purpose |
|---|---|
| `internal/domain/{version,platform,catalog,ownership,plan,inventory,failure}/` | Domain packages, each with `README.md` and tests |
| `internal/usecase/` + `internal/usecase/{bootstrap,scan,conflicts,planning,execute,doctor,status,summary}/` | Use cases and their ports, each with `README.md` |
| `internal/adapter/` + `internal/adapter/{execrunner,platform,statedir,sqlite,catalogyaml,brew,detect,fsprobe,installer,report,cli,tui}/` | Adapters, each with `README.md` |
| `internal/adapter/catalogyaml/embedded/catalog.yaml`, `.../profiles/{agentic-factory,base-dev}.yaml` | Catalog v1 and profiles |
| `internal/adapter/report/schema/report.v1.json` | Report schema v1 |
| `internal/testkit/` (+ `README.md`) | Test-only helpers: builds `fakecmd`, temp PATH/HOME/state dir, fixture loader, invocation recorder. Imported only from `_test.go` files |
| `testdata/fakecmd/main.go` | Fake multi-call command (`brew`, `xcode-select`, `mdfind`, `plutil`, `npm`, `ldd`, `file`) driven by a JSON fixture; records every invocation |
| `testdata/fixtures/*.json`, `testdata/catalogs/integration/` | Machine fixtures for e2e tests; a one-entry catalog (`hello`) for the real-Homebrew test |
| `cmd/facman/e2e_test.go`, `cmd/facman/integration_test.go` | End-to-end tests (fake brew) and `integration`-tagged real-Homebrew tests |

### Files to modify

| Path | Change |
|---|---|
| `cmd/facman/main.go`, `main_test.go`, `README.md` | Wire the CLI adapter; keep `--version`/`--about` behaviour and tests |
| `cmd/facman/wire.go` (new) | `options` struct (platform probe, UID, known brew prefixes, app dirs, `/etc/os-release` and `/proc/version` paths, installer URL, clock, user, stdio) with `defaultOptions()`; builds every adapter. The only place tests inject environment facts **[review]** |
| `Makefile` | `CGO_ENABLED=0` in `build`; add `build-all` and `integration` targets |
| `.github/workflows/ci.yml`, `.github/README.md` | Integration step uses `make integration`; macOS sets `FACMAN_INTEGRATION_MUTATE=1` |
| `go.mod`, `go.sum` | New dependencies (below) |
| `internal/README.md`, `internal/domain/README.md` | List new layers and packages |
| `README.md`, `docs/technical-details.md`, `docs/product-details.md`, `docs/architecture-decision-record.md`, `user-docs/getting-started.md`, `user-docs/configuration.md` | Describe the shipped commands, architecture and decisions |

### Dependencies (pure Go, no cgo)

| Module | Use |
|---|---|
| `github.com/spf13/cobra` | CLI command tree (design §4) |
| `github.com/charmbracelet/bubbletea`, `bubbles`, `lipgloss` | TUI (design §8); latest stable major at implementation time, recorded in the ADR |
| `modernc.org/sqlite` | Inventory (FR-060) |
| `go.yaml.in/yaml/v3` | Catalog and profile YAML with line numbers (maintained successor of `gopkg.in/yaml.v3`) |
| `golang.org/x/term` | Terminal detection for the headless consent prompt |
| `golang.org/x/sync` | `errgroup` for parallel detectors |
| `github.com/creack/pty` (tests only) | Drive the headless consent prompt through a pseudo-terminal |
| `github.com/santhosh-tekuri/jsonschema/v6` (tests only) | Validate `--json` output against report schema v1 |

## Breaking Changes

- **API:** none (no public API yet).
- **CLI:** `facman` with no arguments currently prints `--about` text; it now opens the TUI when stdout is a terminal. When stdout is not a terminal, it prints the `--about` text and a hint to use a headless command, and exits 0 **[spec]**, so piping the binary keeps working. `--version` and `--about` are unchanged. The unknown-flag exit code changes from 2 to 64 (usage error, Appendix H).
- **Config / schema:** new state DB and log directory; no previous version exists.

## Edge Cases and Error Handling

| Case | Behaviour |
|---|---|
| Exit-code precedence | Usage (64) is decided before any I/O. Then bootstrap fatals (4), then validation (3: catalog, profile, `--with`), then the run: 1 if any selected package failed, was excluded for `conflict`, was a dependent of an excluded/failed package, or had incomplete detection; else 0. `doctor` alone returns 2. |
| Empty plan | Prints "Nothing to install", no prompt, no rows, exit 0. |
| Homebrew missing at `apply` | Plan = installer step + installs for packages detected absent. After the installer step, `facman` re-locates brew, applies `shellenv`, re-detects and re-runs the conflict pre-flight. It runs only commands that were in the consented list and are still needed; packages now present are skipped, new conflicts are excluded (exit 1). It never runs a command that was not shown. |
| Installer download fails | 3 retries with backoff, then exit 4 naming the URL and the fix (install Homebrew by hand). |
| Detector timeout or failure (brew broken, `mdfind` hangs) | That detector reports `unknown` with the reason. A package whose detection is incomplete is shown with a warning and is **never planned** (safe default), listed as "not installed: detection incomplete", exit 1 on `apply`. |
| `brew info` returns invalid JSON | Same as detector failure, with the parse error in the reason. |
| Concurrent `facman` runs | `apply` and the TUI take an exclusive lock (`<state dir>/facman.lock`, `flock`). A second mutating run exits 4 naming the holder PID. Read-only commands do not block: when the lock is held they skip writing detections and print a warning. |
| Crash or kill mid-run | Next launch (holding the lock) marks `in_progress` rows `interrupted`, re-detects, and the next `apply` proposes what is still missing (I1-11). |
| `Ctrl+C` during Run (TUI or headless) | Asks for confirmation in the TUI; headless takes the first signal. Stops after the current command finishes; never kills a command midway. Remaining actions are not started and not recorded; exit 1. A second signal while waiting is ignored, the operations log stays consistent. |
| Install command fails | Row finished with its exit code and status `failed`; its dependents are skipped ("not installed: dependency failed"); independent packages continue; exit 1. No automatic retry. |
| State dir not writable, or disk full | Exit 4 naming the path. |
| State DB from a newer `facman` (`schema_version` higher than known) | Exit 4: "state was written by a newer facman"; never migrates down. |
| Corrupt DB | Exit 4 with the path and a hint to move the file aside; `facman` never deletes it. |
| UID 0 | Every command except `--version`, `--about`, `--help` exits 4 with the fix (run as the developer user). |
| Brew prefix owned by another user | `scan`/`plan`/`status`/`doctor` report it; `apply` and the TUI Run exit 4 (multi-user brew is a non-goal). |
| Linux prerequisites missing | Reported with the exact apt/dnf command; `apply` exits 4. Unknown distro (no `ID`/`ID_LIKE` match for debian/ubuntu or fedora/rhel) with missing prerequisites: exit 4. |
| macOS without CLT | Reported; git detection skips `/usr/bin/git`; the Homebrew installer installs CLT itself. If brew exists but CLT is missing, a warning is shown and installs proceed (bottles do not need CLT). |
| `--catalog` dir missing, unreadable or invalid | Exit 3 with `file:line:col`. A git URL → exit 64 "not supported until a later increment". |
| Unknown `--profile` or `--with` ID | Exit 3 naming the ID and the known ones. `--with` of a non-optional package is accepted and has no effect. `--with <choice-group-id>` without a member → exit 3 listing the members. Two `--with` members of one choice group → exit 3. |
| Choice group already satisfied | If any member is present, the group is satisfied and nothing is proposed (`satisfied_by_any_present`). |
| Empty or whitespace `--profile` value | Exit 64. |
| Node keg-only `node@24` | Never `brew link --force` (D-3); `{brew_prefix}/opt/node@24/bin` goes first on children's PATH and Summary prints the profile line. |
| Package already trusted/tapped | The tap or trust step is omitted from the plan; nothing recorded. |
| Version command hangs or prints garbage | Version is `unknown`; the package is still classified by presence. Version commands run with a 5 s timeout and never through a shell. |
| Secrets in logs | Child environments are logged by variable name only. |
| `HOME` unset or state path unresolvable **[review]** | Exit 4 naming the missing variable. |
| Profile resolves to zero packages, or every package is already present **[review]** | Same as empty plan: "Nothing to install", exit 0. |
| Same `--with` or `--profile` given twice **[review]** | De-duplicated silently. |
| `--json` with `apply` **[review]** | The command list and `Proceed? [y/N]` go to stderr; stdout carries only the report. Non-terminal stdin is still exit 64. |
| Signal during the Homebrew installer download **[review]** | Download aborted, temp file removed, nothing recorded (no command ran yet), exit 1. |
| Log file cannot be written mid-run **[review]** | The command is not started; its operation row is finished `failed` with the error; dependents skipped; exit 1. |
| Terminal narrower than 80 columns or resized **[review]** | The TUI re-renders on `tea.WindowSizeMsg`, truncates long cells with `…`, and never panics; `i` shows full text. |
| Catalog entry with no install block for any platform **[review]** | Valid; never offered (not shown as wanted). |

## Success Criteria and Acceptance Criteria

Increment 1 is done when every criterion I1-1 to I1-15 passes in CI and I1-16 is handed to J. They are copied from the PRD Delivery plan; clarifications from this spec are in brackets.

- [x] **I1-1 Identity.** `facman --version` and `--about` print "Agentic Software Factory Manager". `make build` produces `bin/facman` with `CGO_ENABLED=0`, and `make build-all` produces darwin and linux binaries for amd64 and arm64 (FR-001 to FR-003).
- [x] **I1-2 Scan.** `facman scan --json` against a fixture machine reports every catalog package with state, provenance, path and version, and validates against report schema v1. The fake `brew` records only read-only invocations. The scan finishes in under 5 s (FR-007, FR-010, FR-011).
- [x] **I1-3 Ownership states.** Table-driven tests produce each of `absent`, `foreign`, `managed`, `adopted`, `drifted`, `duplicate`, `conflict` and `unavailable-here` from detections plus inventory. With `xcode-select -p` failing, `/usr/bin/git` is never executed, and OS-bundled git, sqlite and jq show as "present (OS)" (FR-011, FR-014, FR-020, FR-061).
- [x] **I1-4 Catalog and profiles.** The embedded catalog loads and validates. `agentic-factory` resolves to its packages plus `base-dev` (git, gh, tmux, visual-studio-code, iterm2 on darwin, sqlite, node, awscli). Optional items are off unless `--with` names them. An unknown ID or conflicting pins exit 3. `--profile` repeats and unions. `--catalog <dir>` replaces the embedded catalog (FR-040 to FR-043, FR-079).
- [x] **I1-5 Bootstrap.** With no Homebrew, `scan` and `plan` report it and install nothing. `apply --profile` lists the installer (temp file, never piped, marked unverified) as the first command, and declining leaves the machine unchanged. UID 0 exits 4 with a fix. Homebrew at `/home/linuxbrew/.linuxbrew` is found when it is not on PATH. glibc below 2.39 warns in `--dry-run` (FR-030, FR-050 to FR-053, FR-055, FR-056). [Known prefixes are injectable in tests so the e2e test needs no root-owned paths. **[review]** After a consented installer step (fake installer served over `httptest`), the `packages` table holds `homebrew` as `managed` with `bootstrap = 1`, later commands see `HOMEBREW_PREFIX` and the brew `bin` on PATH from `brew shellenv`, and only consented commands run.]
- [x] **I1-6 Plan and dry run.** `facman plan --profile agentic-factory` prints the exact ordered commands, with dependencies first (for example `awscli` before `session-manager-plugin` when it is selected). `apply --dry-run` prints the same list and writes no `packages` or `operations` rows, and the fake `brew` records no mutating call (FR-025, FR-028, FR-074).
- [x] **I1-7 Apply and audit.** After consent, `apply --profile` runs exactly the listed commands in order. Each is recorded `in_progress` before it runs and with its exit code and log path after. `consent_by` is `<os user>@<RFC 3339 time>`, and installed packages are `managed`. A second `apply` proposes and runs nothing and exits 0 (FR-027, FR-032, FR-076 behaviour for `--profile`).
- [x] **I1-8 Hands off foreign.** Across all tests, no command touches a `foreign`, `duplicate` or `drifted` package, and no `upgrade` or `uninstall` is ever issued (FR-012, FR-013). [Enforced by a shared invocation-log assertion run at the end of every e2e test.]
- [x] **I1-9 Conflict refusal.** A dangling symlink at `{brew_prefix}/bin/claude` makes `claude-code` a `conflict`, classed `orphan`, with full evidence in `i` and in `--json`. It gets no brew command, the other packages install, and the run exits 1. `doctor` lists it and exits 2. No recorded invocation in the whole suite contains `--force` or `--overwrite` (FR-015 to FR-018). [**[review]** `doctor` on a clean machine exits 0, and with `stablyai/orca` untapped or untrusted it reports the exact `brew trust --cask stablyai/orca/orca` fix and exits 2.]
- [x] **I1-10 Tap trust.** With Orca in the plan, Review and `plan` show `brew tap stablyai/orca` and `brew trust --cask stablyai/orca/orca` as separate steps before its install, and the grant is recorded in `tap_trust`. Whole-tap trust is never requested, and `HOMEBREW_NO_REQUIRE_TAP_TRUST` is never set in any child environment (FR-047).
- [x] **I1-11 Crash recovery.** A run killed after an `in_progress` row is reconciled on next launch (the row is marked interrupted and the package re-detected), and the next `apply` finishes the remaining work (FR-032).
- [x] **I1-12 Exit codes and usage.** Exit codes are 0, 1, 3, 4 and 64 as in Appendix H. `--profile` with no value, a missing `--profile` on `plan` or `apply`, and `apply` with non-terminal stdin all exit 64 with a hint (FR-006, FR-075). [Also: a deferred command name exits 64.]
- [x] **I1-13 TUI.** Golden tests cover Scan → Plan → Review → Run → Summary. Required missing items are pre-checked and optional ones are not. Foreign items cannot be selected. `unavailable-here` rows are greyed with a reason (Paperclip: npm backend), and `conflict` rows show evidence on `i`. Only `Enter` on Review consents, `Esc` goes back, and `q` quits with nothing changed. Summary shows sign-in next steps (FR-026, FR-044, FR-112). [**[review]** An e2e test on a Linux fixture with no `DISPLAY` shows every `gui` item as `unavailable-here` in `plan --json` and lists it as skipped with a warning; a WSL fixture shows the `winget` hint (FR-044, FR-045).]
- [x] **I1-14 Real Homebrew (integration, macOS CI).** `scan --json`, `plan` and `doctor` succeed against the runner's Homebrew, and installing one absent small formula records it as `managed`. [Formula: GNU `hello`, through a one-entry `--catalog`. The install runs only when `FACMAN_INTEGRATION_MUTATE=1`, which CI sets on macOS only; elsewhere the test skips. Read-only checks skip when no Homebrew is found or when running as root.]
- [x] **I1-15 Quality gates.** `make check` passes, coverage is at least 80% with new code fully covered, every new folder has a README, and the root README, `docs/technical-details.md`, `docs/product-details.md` and `user-docs/` describe the shipped commands.
- [ ] **I1-16 Field check (manual, J, after shipping).** *Handed over to J (2026-11-14): procedure in [user-docs/field-check.md](../../../user-docs/field-check.md); evidence for I1-1 to I1-15 is in [docs/technical-details.md](../../../docs/technical-details.md#acceptance-evidence).* `facman scan --json` on 3 to 5 lab Macs, with findings fed back into the catalog's provenance rules (Phase 0). Does not gate the increment.

### FR → acceptance criteria traceability

| FR | Criteria |
|---|---|
| FR-001, FR-002, FR-003 | I1-1 |
| FR-004 (partial), FR-006, FR-075 | I1-12 |
| FR-007 (partial) | I1-2, plus `--json` schema tests for `plan`, `apply`, `status`, `doctor` (T-F1, T-F5) |
| FR-010, FR-011 | I1-2, I1-3 |
| FR-012, FR-013 (partial), FR-014 | I1-3, I1-8 |
| FR-015, FR-016, FR-017 (partial), FR-018 | I1-9 (conflict and tap-trust parts) |
| FR-020 | I1-3 |
| FR-025, FR-028, FR-074 (partial) | I1-6 |
| FR-026 (partial) | I1-13 |
| FR-027, FR-032, FR-076 (partial) | I1-7, I1-11 |
| FR-029 (partial), FR-031 | I1-7, I1-14, sudo hand-off unit tests (T-C1, T-G4) |
| FR-030 (partial), FR-050 to FR-056 (partial/full as above) | I1-5 (including the `homebrew` bootstrap record and `shellenv` for children) |
| FR-040 to FR-043, FR-079 (partial) | I1-4 |
| FR-044, FR-045 (partial) | I1-13 (TUI and the Linux/WSL e2e fixtures), platform and classification unit tests (T-A6, T-A10) |
| FR-047 | I1-10 |
| FR-060 to FR-062 | I1-3, I1-7, I1-11 |
| FR-112 (partial) | I1-13 |
| NFRs | I1-2 (scan time), I1-15, T-F6 |

### Quality gates

- `PATH=$HOME/go/bin:$PATH make check` green before every commit; CI green on Linux and macOS.
- `make cover` ≥ 80% overall; domain and use case packages ≥ 90% (Step 11 target); new code fully covered.
- `go test -race -tags integration ./...` passes everywhere: skips cleanly without Homebrew, read-only with Homebrew, mutating only with `FACMAN_INTEGRATION_MUTATE=1`.
- No new folder without a `README.md`; docs updated in the same commits.

## Risks and Mitigation

| Risk | Mitigation |
|---|---|
| Real `brew info --json=v2` shapes differ from fixtures | Fixtures are captured from real Homebrew output (documented in research.md); I1-14 runs against real Homebrew on macOS CI. |
| `mdfind` slow or Spotlight disabled | 10 s detector timeout, `/Applications` fallback scan, `unknown` reported instead of failing. |
| Bubble Tea golden tests are brittle (width, colour) | Render at a fixed width with an ASCII colour profile; golden files updated with `-update` only. |
| Parallel workstreams conflict in `go.mod` | Dependencies added by the workstream that first needs them; integration branch runs `go mod tidy`; see plan.md. |
| Installing a real formula on macOS CI mutates the runner | Opt-in env var, ephemeral runner, test cleans up only what it installed. |
| Catalog facts unverified (Orca bundle ID, Linux casks) | Kept as data in the catalog; I1-16 field check feeds corrections; no code change needed. |
| Homebrew 6 tap-trust CLI changes | Commands built in one place (`adapter/brew`); I1-14 exercises a real Homebrew. |
| Scope creep from the full PRD | Deferred table above; anything outside it is rejected in review. |

## Open Questions

| # | Question | Owner | Resolution path / default in force |
|---|---|---|---|
| OQ-1 to OQ-11 | PRD Open Questions 1 to 11 (identities, minimum versions, Node managers, Claude Code channel, adoption default, multiple factories, distribution, WSL host apps, Linux desktops, config-list catalog pinning, suppression scope) | J | PRD recommended defaults are in force. Those that touch Increment 1 (1, 2, 4, 6, 8, 9) are implemented as the defaults say; the rest affect later increments. |
| OQ-13 | Owner and date for Apple signing, notarization and the Homebrew tap | J | Not needed for Increment 1 (unsigned binaries with checksums for lab testing). |
| S-1 | Orca's real bundle ID (`dev.orca.app` is a guess) | J, via I1-16 field run | Catalog data; PATH detection of `orca` still works if the bundle ID is wrong. |
| S-2 | Whether the Linux casks the catalog lists (claude-code, copilot-cli, codex, antigravity-cli, gcloud-cli, t3-code) install on Linux Homebrew as the design's matrix says | Implementer (Step 3), confirmed by J's lab runs | Catalog data; a wrong entry gives a failed install (exit 1) with the brew error, never a hang. |
| S-3 | Bubble Tea major version (v1 vs v2) | Implementer (Step 3) | Latest stable at implementation, recorded in the ADR. |

## Timeline and Milestones

| Milestone | Content | Exit |
|---|---|---|
| M1 Foundation | Workstream A (domain model, fakecmd, Makefile) | `make check` green; domain fully tested |
| M2 Parallel build-out | Workstreams B, C, D, E | Each lands with tests and READMEs |
| M3 Front ends | Workstreams F (CLI, report, e2e) and G (TUI) | I1-1 to I1-13 green |
| M4 Integration and docs | Workstream H | I1-14, I1-15 green; ready for review (dev-flow Step 4 onwards) |

## References

- Source PRD: [specs/261114-software-factory-manager/software-factory-manager-PRD.md](software-factory-manager-PRD.md), especially *Delivery plan* and Appendices A to L
- [INTENT.md](../../../INTENT.md), [AGENTS.md](../../../AGENTS.md)
- Design: [docs/design/software-factory-installer-design.md](../../../docs/design/software-factory-installer-design.md) (§3, §3a, §3b, §4, §4a, §5, §6, §7, §8)
- [docs/architecture-decision-record.md](../../../docs/architecture-decision-record.md)
- Spec files: [research.md](research.md), [data-dictionary.md](data-dictionary.md), [architecture.md](architecture.md), [plan.md](plan.md), [tasks.md](tasks.md), [implementation-notes.md](implementation-notes.md), [status.md](status.md)

## Spec review log (2026-11-14)

`/review-spec` was run unattended (dev-flow `/implm-frm-prd` Step 2) while J was away. Every gap was resolved here from the PRD, INTENT.md, AGENTS.md and the design's safest-default and consent-first rules; decisions R-1 to R-9 are also in [implementation-notes.md](implementation-notes.md#spec-review-decisions-2026-11-14). J can overturn any of them.

**Verdict before:** Needs revision (no Fail, four Warnings). **Verdict after:** Implementation-ready.

| Dimension | Before | After | Note |
|---|---|---|---|
| Acceptance criteria | ⚠️ Warn | ✅ Pass | I1-1 to I1-16 numbered and testable, but three in-scope behaviours had no criterion: the `homebrew` bootstrap record and `shellenv` for children (FR-053, FR-055), `doctor` exit 2 for an untrusted needed tap (FR-018), and GUI `unavailable-here` on headless Linux and WSL (FR-044, FR-045). Added as bracketed clarifications to I1-5, I1-9, I1-13 and to the traceability table |
| Technical approach | ⚠️ Warn | ✅ Pass | Three choices were implicit: how the TUI hands a sudo command to `tea.ExecProcess` while `execute` runs it (R-4), how e2e tests fake darwin/WSL on a Linux runner (R-3), and how D can implement ports before E lands (R-5). Now decided |
| Component coverage | ⚠️ Warn | ✅ Pass | `cmd/facman/wire.go` and its injected options were missing from the file list. Added; every file and folder to create or change is named |
| Edge case handling | ⚠️ Warn | ✅ Pass | External failures, concurrency, crashes and permissions were covered. Missing: unset `HOME`, empty resolution, duplicate flags, `--json` with the prompt, signal during download, log write failure, narrow terminals, entries with no install block. Added to Edge Cases |
| Open questions | ✅ Pass | ✅ Pass | OQ-1 to OQ-11 and OQ-13 (J, defaults in force), S-1 to S-3 with owners |
| Out-of-scope clarity | ✅ Pass | ✅ Pass | Deferred-requirements table by increment |
| Status initialization | ✅ Pass | ✅ Pass | Phase 0 was In Progress, the rest Not Started; updated after this review |

**Resolutions:**

1. **R-1** Added acceptance coverage for the `homebrew` bootstrap record (`managed`, `bootstrap = 1`) and `brew shellenv` reaching children (I1-5).
2. **R-2** Added `doctor` clean → 0 and untrusted `stablyai/orca` → 2 with the exact fix (I1-9).
3. **R-3** Environment facts (platform, UID, prefixes, app dirs, file paths) are injected only through `cmd/facman/wire.go` options, so e2e tests can run darwin, Linux-no-display and WSL fixtures on any runner; nothing is exposed as a flag or env var (extends D-11).
4. **R-4** TUI sudo hand-off: the TUI wraps the runner so an interactive command sends an exec request to the Bubble Tea program, which runs it with `tea.ExecProcess` and returns the exit code on a channel; `execute` blocks on it. Headless mode runs the same command with inherited stdio.
5. **R-5** T-E1 (ports and fakes) lands in workstream A's commit, so B, C, D and E start in parallel against fixed port interfaces, and adapters carry compile-time interface assertions from the start.
6. **R-6** The root command takes `--profile` (default `agentic-factory`), `--with` and `--catalog` for the TUI.
7. **R-7** Eight extra edge cases specified (Edge Cases, marked **[review]**).
8. **R-8** `--json` with `apply`: prompt and command list on stderr, report on stdout.
9. **R-9** Linux-no-display and WSL fixtures added to the e2e suite (I1-13), so FR-044/FR-045 are tested beyond unit level.

