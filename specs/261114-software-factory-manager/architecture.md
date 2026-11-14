# Architecture: software-factory-manager, Increment 1

**Feature:** software-factory-manager (`facman`)
**Date:** 2026-11-14
**Status:** Reviewed (2026-11-14)

## Architecture Overview

`facman` is one static Go binary. Increment 1 maps the design's components ([design §4](../../docs/design/software-factory-installer-design.md#4-architecture)) onto the Clean Architecture layers in [AGENTS.md](../../AGENTS.md). Dependencies point inward only:

```
cmd/facman ──► internal/adapter/* ──► internal/usecase/* ──► internal/domain/*
     │                                        ▲
     └────────────── wiring only ─────────────┘  (cmd may import any inner layer)
```

- **Domain** holds the rules that make `facman` safe: ownership classification, profile resolution, plan invariants (no `--force`, item trust only, commands only for `absent` packages), consent strings, exit-code kinds. No I/O, no imports from other layers.
- **Use cases** orchestrate: bootstrap check → scan (detect, reconcile, classify) → conflict pre-flight → plan → consent → execute → summary. They declare the ports they need (see [data-dictionary.md §3](data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed)).
- **Adapters** do the I/O: subprocesses, filesystem, SQLite, YAML, HTTP download, and the two driving adapters (CLI and TUI).
- **`cmd/facman`** builds concrete adapters, injects them into use cases and hands control to the CLI adapter. No logic, no globals, no `init()`.

## Component Architecture

### Package map

| Package | Layer | Responsibility | Depends on |
|---|---|---|---|
| `internal/domain/appinfo` | domain | Name, title, version text (exists) | — |
| `internal/domain/version` | domain | Version parse and compare | — |
| `internal/domain/platform` | domain | `Platform`, `PlatformKey`, `BootstrapReport`, `Finding` | `version` |
| `internal/domain/catalog` | domain | `CatalogEntry`, `Catalog`, `Profile`, validation, profile resolution (union, pins, `--with`, choice picks), per-platform install selection | `platform`, `version`, `failure` |
| `internal/domain/ownership` | domain | `Detection`, `OwnershipState`, `PackageStatus`, `Classify` | `catalog`, `inventory`, `plan` (Conflict type), `version` |
| `internal/domain/plan` | domain | `Command`, `Action`, `Plan`, `Conflict`, `TapTrustStep`, `Consent`, ordering and invariants | `catalog` |
| `internal/domain/inventory` | domain | `Record`, `Operation`, `TapTrustGrant`, statuses | `plan` (Command) |
| `internal/domain/failure` | domain | Typed errors → exit codes | — |
| `internal/usecase/bootstrap` | usecase | Build `BootstrapReport`; decide fatal findings per command | domain |
| `internal/usecase/scan` | usecase | Run detectors in parallel with timeouts, reconcile inventory (interrupted ops, drift), classify, persist detections | domain |
| `internal/usecase/conflicts` | usecase | Link-path pre-flight and owner classification for brew items | domain |
| `internal/usecase/planning` | usecase | Desired set + statuses + bootstrap → `Plan` (installer, tap, trust, install actions; exclusions) | domain |
| `internal/usecase/execute` | usecase | Consent → run actions with the operations log, dependents skipping, re-plan after Homebrew bootstrap, stop-after-current | domain |
| `internal/usecase/doctor` | usecase | Whole-catalog conflict + catalog-tap trust scan → findings, exit 0/2 | domain, `usecase/conflicts` |
| `internal/usecase/status` | usecase | Managed/adopted/foreign/drifted view | domain, `usecase/scan` |
| `internal/usecase/summary` | usecase | Results, "not installed" reasons, sign-in next steps, shell lines | domain |
| `internal/adapter/execrunner` | adapter | `os/exec` runner: argv only, env sanitizing, timeouts, log tee, interactive hand-off; system clock and user | usecase ports |
| `internal/adapter/platform` | adapter | OS/arch/WSL/display/glibc/distro/UID probe | domain |
| `internal/adapter/statedir` | adapter | Appendix J paths, lock, log naming, 90-day prune | — |
| `internal/adapter/sqlite` | adapter | `modernc.org/sqlite` inventory, numbered migrations | domain |
| `internal/adapter/catalogyaml` | adapter | Embedded catalog v1 and profiles; `--catalog <dir>`; YAML with `file:line:col` errors | domain |
| `internal/adapter/brew` | adapter | Locate brew, `shellenv`, `brew info --json=v2` parsing, brew detector, `brew-formula`/`brew-cask` backends, tap/trust state | domain, execrunner (via port) |
| `internal/adapter/detect` | adapter | PATH, app-bundle and npm detectors; CLT check; OS-bundled rule; provenance globs | domain |
| `internal/adapter/fsprobe` | adapter | Conflict evidence and owner classification on the filesystem | domain |
| `internal/adapter/installer` | adapter | Homebrew installer download (retries) and command | domain |
| `internal/adapter/report` | adapter | Report schema v1 (embedded) and JSON encoding | domain, use case result types |
| `internal/adapter/cli` | adapter (driving) | cobra command tree, flags, exit-code mapping, text output, `[y/N]` prompt, signal handling, launching the TUI | use cases, `report`, `tui` |
| `internal/adapter/tui` | adapter (driving) | Bubble Tea model: Scan → Plan → Review → Run → Summary | use cases |
| `internal/testkit` | test support | Build `testdata/fakecmd`, temp HOME/PATH/state, invocation-log assertions | — (imported only by `_test.go`) |
| `cmd/facman` | entry | Wiring (`main.go`, `wire.go` with injectable `options`: platform probe, UID, known prefixes, app dirs, file paths, installer URL, clock, user, stdio); keeps `var version` for `-ldflags` | everything |

### Driving adapters live in `internal/adapter`, not `cmd`

AGENTS.md says `cmd/` holds wiring and flags. The cobra tree and the TUI contain presentation logic that needs tests (exit codes, prompt, golden screens), so they live in `internal/adapter/cli` and `internal/adapter/tui`, and `cmd/facman/main.go` only constructs adapters and calls `cli.Execute(ctx, args, stdio, deps)`. This keeps `cmd` thin and makes the front ends unit-testable (AD-2).

## Layer Responsibilities

- **Domain must:** encode every safety rule as a pure function or invariant so it is tested exhaustively by table-driven tests (classification, plan validation, profile union, exit-code precedence).
- **Use cases must:** never import adapters; take `context.Context` first on anything that does I/O through a port; return `failure.Error` kinds; be fully testable with in-memory fakes.
- **Adapters must:** contain no business rules beyond translating I/O (parsing, encoding); never build shell strings; wrap errors with context.
- **cmd must:** only wire. No flags parsed outside `adapter/cli`, except that `var version` stays in `main` for `-ldflags`.

## Data Flow

### Every command (except `--version`, `--about`, `--help`)

1. `cli` parses flags; usage errors → exit 64 before any I/O.
2. `bootstrap.Check` → `BootstrapReport` (platform, UID, CLT, prerequisites, Homebrew location and owner, `shellenv`, Node). UID 0 → exit 4. The runner's child environment is now `os.Environ()` + `shellenv` + `{brew_prefix}/opt/node@24/bin` prepended to PATH, minus `HOMEBREW_NO_REQUIRE_TAP_TRUST*`.
3. `statedir` opens the state dir; `PruneLogs`; `sqlite` opens and migrates the DB. Mutating commands take the lock (`TryLock` for read-only ones).
4. `catalogyaml` loads the embedded catalog or `--catalog <dir>`; `Catalog.Validate` → exit 3 on errors.
5. `scan.Run`: when holding the lock, `MarkInterrupted` in-progress operations; run detectors in parallel (`errgroup`, 10 s each); classify each catalog entry against inventory records, availability for this platform and (for planned brew items) conflicts; save the detection snapshot (only when holding the lock).

### `facman scan`

Steps 1–5 → text table or report v1 → exit 0 (or 4 for fatal bootstrap findings).

### `facman plan --profile P [--with X] [--catalog D]`

Steps 1–5 → `catalog.Resolve` (exit 3 on unknown IDs/pins) → `conflicts.Check` for selected `absent` brew items → `planning.Build` → `plan.Validate` (invariants; a violation is a bug and returns exit 1 with "internal error") → print ordered commands with sudo/unverified markers, exclusions and warnings (glibc) → exit 0.

### `facman apply --profile P [--dry-run]`

Same as `plan`, then:

- `--dry-run`: print and exit 0 (the runner is never called for a mutating command; no rows).
- empty plan: "Nothing to install", exit 0.
- stdin not a terminal → exit 64. Else prompt `Proceed? [y/N]` → decline exits 0 with no rows.
- Fatal apply-time findings (Linux prerequisites, foreign brew prefix owner, lock held) → exit 4 **before** prompting.
- `execute.Run(plan, consent)`: for each action in order: skip if a dependency failed or was excluded; `StartOperation` (`in_progress`, commands, consent, log path); run each command (interactive when `NeedsSudo`); `FinishOperation`; on success upsert `packages` (`managed`) or `tap_trust`. After the `bootstrap-homebrew` action: re-locate brew, refresh `shellenv`, re-scan and re-check conflicts for the remaining actions, and drop actions whose package is now present or in conflict (exclusion recorded). Never adds commands.
- `summary.Build` → results, "not installed" reasons, sign-in next steps, shell lines → exit 0 or 1.

### `facman status`

Steps 1–5 → list `managed`, `adopted`, `foreign`, `drifted`, `duplicate` packages with provenance → exit 0.

### `facman doctor [--catalog D]`

Steps 1–4 → scan → `conflicts.Check` for **every** catalog brew item available on this platform that is `absent` → tap/trust state of every catalog tap item → findings → exit 2 if any conflict or untrusted needed tap, else 0.

### `facman` (TUI)

Steps 1–4 → TUI starts on Scan (scan runs as a `tea.Cmd`) → Plan (profiles from the root `--profile`, default `agentic-factory`; `--with` and `--catalog` as for `plan`) → Review (`Enter` = consent) → Run → Summary.

**Run and sudo hand-off (R-4).** `execute.Run` runs in a goroutine started by a `tea.Cmd`, with a `Progress` sink that sends `tea.Msg`s to the program. The TUI wraps the system runner: a non-interactive command runs directly; an interactive (`NeedsSudo`) command sends an `execRequest{cmd, reply chan Result}` message, the model returns `tea.ExecProcess(cmd, …)`, and the callback writes the exit code to `reply`, on which `execute` is blocked. So the terminal is handed to the child and `facman` never sees the password. Headless, the same command runs with inherited stdio.

## Sequence Diagrams

### Headless apply with Orca and a conflict

```
User        cli           bootstrap   scan        conflicts   planning    execute       brew(fake/real)   sqlite
 |  apply --profile agentic-factory
 |---------->|  Check()      |           |            |           |            |               |            |
 |           |-------------->| brew found, shellenv    |           |            |               |            |
 |           |  Run()                    |            |           |            |               |            |
 |           |-------------------------->| brew info --json=v2 --installed (read-only) ------->|            |
 |           |                           | PATH/bundle/npm detectors in parallel               |            |
 |           |                           | reconcile + classify ------------------------------------------->| read
 |           |  Check(selected absent)                |           |            |               |            |
 |           |--------------------------------------->| /opt/homebrew/bin/claude: dangling symlink → orphan |
 |           |  Build()                                           |            |               |            |
 |           |--------------------------------------------------->| tap, trust, installs; claude-code excluded (conflict)
 |<----------| print commands, "Proceed? [y/N]"                   |            |               |            |
 |  y        |  Run(plan, consent "j@2026-11-14T19:00:00Z")                    |               |            |
 |---------->|---------------------------------------------------------------->| StartOperation(in_progress) ------>|
 |           |                                                                 | brew tap stablyai/orca ---->|      |
 |           |                                                                 | FinishOperation(0) -------------->|
 |           |                                                                 | brew trust --cask stablyai/orca/orca  |
 |           |                                                                 | RecordTapTrust ------------------>|
 |           |                                                                 | brew install --cask stablyai/orca/orca|
 |           |                                                                 | UpsertRecord(managed) ----------->|
 |<----------| Summary: claude-code not installed: conflict (evidence, fix hint); exit 1
```

### Crash recovery

```
run 1: StartOperation(id=7, in_progress) → brew install gh … SIGKILL
run 2: Lock() → MarkInterrupted() → op 7 status=interrupted → scan re-detects gh
       ├─ gh found through brew-formula (the op's backend) → packages row written as managed (AD-8); nothing proposed for gh
       └─ gh not found → stays absent → this apply proposes `brew install gh` again
```

AD-8 resolves the corner case: if an interrupted `install` operation's package is now detected through the backend that operation used, the reconcile step records it as `managed` (the consented install completed even though the process died), and logs the reconciliation on the interrupted row's log. If it is not detected, it stays `absent` and the next `apply` proposes it again.

## Integration Points

| External | How | Read-only during scan/plan? |
|---|---|---|
| Homebrew (`brew`) | argv subprocess via `execrunner`; `HOMEBREW_NO_AUTO_UPDATE=1`, `HOMEBREW_NO_ENV_HINTS=1` for read calls | Yes |
| `xcode-select`, `mdfind`, `plutil`, `getconf`/`ldd`, `file`, `npm` | argv subprocesses, 5–10 s timeouts | Yes |
| Homebrew installer URL | `net/http` download with retries | Only during `apply` |
| Filesystem | state dir, `Lstat`/`Readlink` for conflicts, `/etc/os-release`, `/proc/version`, `/Applications` | Yes (except state dir) |
| SQLite | `modernc.org/sqlite` | Writes detections when holding the lock |
| Terminal | `golang.org/x/term`, Bubble Tea, `tea.ExecProcess` | — |

## Architectural Decisions

| ID | Decision | Rationale | Alternatives rejected |
|---|---|---|---|
| AD-1 | Packages per concept in each layer (see map) | AGENTS.md: small packages named for what they provide | One big `domain` package (hard to navigate); `util` packages (forbidden) |
| AD-2 | CLI (cobra) and TUI (Bubble Tea) are driving adapters in `internal/adapter/{cli,tui}`; `cmd/facman` only wires | Keeps `cmd` thin and front ends testable | Cobra tree in `cmd/facman` (untestable logic in `main`) |
| AD-3 | cobra for the command tree, with `SilenceUsage`/`SilenceErrors` and our own exit-code mapping; flag errors mapped to 64 | Design §4 names cobra; the Increment 3 short-flag normalizer runs before cobra parses | `flag` stdlib (no subcommands), urfave/cli |
| AD-4 | Ports declared in the consuming use case package; small per-consumer interfaces | AGENTS.md rule | A shared `ports` package |
| AD-5 | `modernc.org/sqlite`, WAL, numbered Go-embedded migrations, `meta.schema_version` | FR-060, no cgo, PRD review decision 23 | JSON file (no transactions), cgo sqlite |
| AD-6 | Catalog and profiles embedded with `embed` under `adapter/catalogyaml/embedded/`, parsed with `go.yaml.in/yaml/v3` node API for line numbers | FR-040, FR-042 partial; YAML errors need `file:line:col` | Go literals (not reviewable as data), JSON |
| AD-7 | Subprocesses as argv only; one runner adapter sanitizes env and handles interactive hand-off | Security NFR, FR-031, I1-10 | `sh -c` strings |
| AD-8 | Interrupted install reconciliation: detected through the operation's backend → `managed`; else `absent` | I1-11; the developer consented to that install, so ownership is honest | Leave it `foreign` (would lose ownership of what `facman` installed) |
| AD-9 | Exclusive `flock` on `<state dir>/facman.lock` for mutating runs; read-only commands skip writes when locked | Concurrent runs must not corrupt the log or double-install | SQLite-only locking (does not stop two brew installs) |
| AD-10 | `node@24` is never `brew link --force`d; its keg `bin` is prepended to children's PATH and Summary prints the profile line | FR-017/I1-9 forbid `--force` in any recorded invocation; FR-053 profile write is deferred | `brew link --force node@24` (breaks I1-9) |
| AD-11 | Report schema v1 is a JSON Schema file embedded in `adapter/report`, validated in tests | FR-007; `export` reuses it later | Go-struct-only contract |
| AD-12 | Testing through fakes: in-memory port fakes for unit tests; one `fakecmd` binary on PATH for e2e; real Homebrew only behind `integration` tag | PRD "How it is tested"; Linux CI makes no real installs | Mocks; network installs in CI |
| AD-13 | Detection makes no network calls (`HOMEBREW_NO_AUTO_UPDATE=1` on read calls; `brew info --installed` is local) | NFR performance and "no network during detection" | — |
| AD-14 | `facman` with no args opens the TUI only when stdout is a terminal; otherwise prints `--about` text and a hint (exit 0) | Keeps `facman \| cat` and CI smoke uses harmless | Always opening the TUI (hangs in pipes) |
| AD-15 | Environment facts are injected only through `cmd/facman/wire.go` options; no test-only flags or env vars in the binary (R-3) | Lets e2e tests run darwin, Linux-no-display and WSL fixtures on any CI runner without exposing hidden knobs | `FACMAN_TEST_*` variables |
| AD-16 | TUI sudo hand-off through an exec-request message and `tea.ExecProcess`, with `execute` blocked on a reply channel (R-4) | One `execute` use case for TUI and headless; credentials never pass through `facman` | Separate TUI executor; suspending the TUI by hand |
