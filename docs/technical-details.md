# Technical Details

This is how `facman` Increment 1 is built: its components, the data flow of each command, the ports between layers, and its dependencies. The rules for changing it are in [AGENTS.md](../AGENTS.md). The working design is in [design/](design/software-factory-installer-design.md). The decisions behind this structure are in the [architecture decision record](architecture-decision-record.md) and in the spec's [architecture.md](../specs/261114-software-factory-manager/architecture.md) (AD-1 to AD-16).

## Layers

```
cmd/facman            wiring only: options (environment facts, streams) → newApp
  │
internal/adapter      driving:  cli (cobra), tui (Bubble Tea v2), session (composes the use cases)
  │                   driven:   execrunner, platform, statedir, sqlite, catalogyaml, brew, detect,
  │                             fsprobe, installer, report
internal/usecase      bootstrap, scan, conflicts, planning, execute, doctor, status, summary
  │                   (each declares its own ports; process holds the shared Cmd/Result values)
internal/domain       appinfo, version, platform, catalog, plan, inventory, ownership, failure
```

Dependencies point inward only. The domain packages import only other domain packages. Every use case imports the domain, plus the value-only `usecase/process` package for runner command and result types (implementation-notes I-8). Adapters implement the ports, and `cmd/facman` wires them together. Nothing uses globals or `init()` side effects.

## Components

### Domain (`internal/domain`)

Pure logic with no I/O. Every safety rule lives here, as a function or an invariant tested by table-driven tests. Coverage is 100%.

| Package | Provides |
|---|---|
| `appinfo` | The name (`facman`), title ("Agentic Software Factory Manager") and the version and about text. It is the only place the name is written. |
| `version` | Parsing and comparing versions and constraints. |
| `platform` | `Platform` (OS, arch, WSL, display, glibc, distro, UID), bootstrap `Finding`s with `Severity` and `BlocksApply`, and the `Command`s that decide fatality. |
| `catalog` | `Catalog`, `Entry`, `Profile`, `Validate` (every problem in one error), `Resolve`/`ResolveSelection` (profile union, pins, `--with`, choice groups, transitive `requires`), `SelectInstall` (per-platform install step), and provenance glob expansion and matching. |
| `plan` | `Command` (argv only), `Action`, `Plan`, `Conflict`, consent, `OrderPackages` (stable requires-first order), `Validate` (allows only `brew install`/`tap`/`trust`; never `--force` or `link`), and `ExcludeWithDependents`. |
| `inventory` | `Record` (managed or adopted), `Operation` (the audit row), tap-trust grants, statuses, and `InstallRef` parsing. |
| `ownership` | `Detection`, `PackageStatus`, `Classify`: absent, managed, adopted, foreign, drifted, duplicate, conflict, unavailable-here, with incomplete detection and OS-bundled rules. |
| `failure` | Typed errors (usage, bootstrap, validation, operation, drift) with a fix and details, mapped to exit codes 64, 4, 3, 1 and 2, and `Worst` for precedence. |

### Use cases (`internal/usecase`)

Application logic. Each package declares the small interfaces it consumes, and is tested with in-memory fakes. Coverage is 97 to 100% per package.

| Package | Does | Key ports |
|---|---|---|
| `bootstrap` | Builds the bootstrap report: platform, UID, CLT, Linux prerequisites, Homebrew location and prefix owner, `shellenv`, Node. Computes the child environment. | `PlatformProbe`, `HomebrewLocator`, `CommandRunner`, `PathLookup` |
| `scan` | Runs the detectors in parallel (10 s each), reconciles interrupted operations (AD-8), classifies every entry and saves a detection snapshot when it holds the lock. | `Detector`, `Inventory`, `StateDir`, `CommandRunner` |
| `conflicts` | Runs the link-path pre-flight for brew items and classifies who owns a conflicting file. | `Backend` (link paths), `PathProbe` |
| `planning` | Turns the desired set, statuses and bootstrap report into a `Plan`: installer, tap, trust and install actions with exclusions and warnings. | `Backend` (`InstallActions`) |
| `execute` | Runs a consented plan. It writes each `operations` row before the action runs, runs the commands, records the result, skips dependents of failures, re-plans after the Homebrew bootstrap, and stops after the current command on cancel. | `Confirmer`, `CommandRunner`, `Inventory`, `StateDir`, `Replanner`, `HomebrewInstaller`, `Progress`, `Clock`, `UserInfo` |
| `doctor` | Runs the whole-catalog conflict check and the tap-trust check for the catalog's own taps. Exit 0 or 2. | `ConflictChecker`, `TapChecker` |
| `status` | Builds the managed, adopted, foreign, drifted and duplicate view, with provenance. | `Inventory` |
| `summary` | Per-package results, "not installed" reasons, sign-in next steps and shell-profile lines. | none |
| `process` | The `Cmd` and `Result` value types that every `CommandRunner` port shares. | none |

### Adapters (`internal/adapter`)

| Package | Role |
|---|---|
| `cli` | cobra command tree, the short-flag normalizer, the exit-code mapping (usage errors are decided before any I/O), text rendering, the `Proceed? [y/N]` prompt on stderr, and signal handling. Deferred commands and flags exit 64, naming their increment. |
| `tui` | The Bubble Tea v2 program: Scan → Plan → Review → Run → Summary. It holds no business logic. Ticks become a `catalog.Selection`, re-resolved by the domain. `Enter` on Review is the only consent, and the consent covers exactly the plan shown. Sudo steps are handed the terminal through `tea.ExecProcess` (AD-16). |
| `session` | Composes the use cases in the right order for every command and for the TUI. It holds the state lock from the scan through the run, and `Machine`, the located brew and child environment, which is updated after the Homebrew bootstrap. |
| `execrunner` | `os/exec` runner. It takes an argument list only (never a shell), sanitizes the environment, applies timeouts, writes a log header (variable names, not values), runs children in their own process group, and builds `Handoff` for interactive commands. Also provides the system clock and user. |
| `platform` | Probes OS, arch, WSL (`/proc/version`, `WSL_DISTRO_NAME`), display, glibc (`getconf`, `ldd`) and distro (`/etc/os-release`). |
| `statedir` | Resolves the state directory per platform, the `flock` lock with holder PID, log naming and 90-day pruning. |
| `sqlite` | Inventory on `modernc.org/sqlite` (pure Go, WAL) with numbered embedded migrations. It refuses newer or corrupt databases (exit 4). |
| `catalogyaml` | Catalog v1 and profiles embedded in the binary, and `--catalog <dir>`. Strict decoding with `file:line:col` errors. |
| `brew` | Locating brew, `shellenv`, parsing `brew info --json=v2`, the brew detector, the `brew-formula` and `brew-cask` backends, link paths, and tap and trust state. |
| `detect` | PATH (with the CLT `git` shim rule), app-bundle (Spotlight, then a directory scan) and npm global detectors. |
| `fsprobe` | Conflict evidence (`ls -l` line, file type, symlink target, dangling) and the owner class of a path. |
| `installer` | Downloads the Homebrew installer to a `0600` temp file (3 tries with backoff), never piped to a shell. Runs it as `/bin/bash <file>`. |
| `report` | Report schema v1 (`schema/report.v1.json`, embedded) and the JSON encoding for every command. |

`internal/testkit` and `testdata/fakecmd` provide the fake multi-call `brew` (and `mdfind`, `getconf`, …) used by the end-to-end tests. They are imported only by tests.

## Data flow

Every command except `--version`, `--about` and `--help` runs the same steps through `session`:

1. **Flags** (`cli`). Usage errors exit 64 before any I/O.
2. **Bootstrap check** (`bootstrap`). UID 0 and other fatal findings exit 4. `BlocksApply` findings (missing Linux prerequisites, a brew prefix owned by another user) exit 4 only for `apply` and the TUI Run.
3. **State** (`statedir`, `sqlite`). Opens the state directory, prunes old logs, and opens and migrates the DB. Mutating runs take the lock (a held lock exits 4 and names the PID).
4. **Catalog** (`catalogyaml`). Loads the embedded catalog or `--catalog <dir>` and validates it. Errors exit 3.
5. **Scan** (`scan`). Runs the detectors in parallel and classifies. Then, per command:

| Command | After the scan |
|---|---|
| `scan` | Render the statuses. |
| `plan` | `catalog.Resolve`, then the conflict pre-flight for absent brew items, then `planning`, then `plan.Validate`. Print the ordered commands. |
| `apply` | As `plan`. Then `--dry-run` prints and stops, an empty plan says "Nothing to install", otherwise the prompt. On `y`, `execute` runs, then `summary`. |
| `status` | `status` view of the records and statuses. |
| `doctor` | Conflict and tap-trust checks for the whole catalog. Exit 0 or 2. |
| `facman` (TUI) | Scan screen. Plan re-resolves the selection on every toggle. Review shows the plan. `Enter` runs `execute` in a goroutine, with progress and hand-off requests on one channel. Then Summary. |

**Homebrew missing at `apply`.** The plan starts with the installer action. After it runs, `execute` asks the `Replanner` (composed from bootstrap, scan and conflicts) to locate brew again, refresh `shellenv`, re-detect and re-check conflicts. Then it only *removes* actions: packages now present become `already-present`, and new conflicts are excluded. It never runs a command that was not shown.

**Crash recovery.** Each action's row is written as `in_progress` before it runs. At the next start, leftover rows are marked `interrupted`, and the package is recorded `managed` if it is now detected through that backend, or left `absent` so the next `apply` proposes it (AD-8).

**Exit codes.** Usage (64) > bootstrap (4) > validation (3) > operation (1) > 0. `doctor` alone returns 2, which ranks below all the others.

## Safety properties and where they are enforced

| Property | Enforced in |
|---|---|
| Nothing mutating runs without consent that names the exact commands | `execute` (consent on every row), `tui` consent equality check, `cli` prompt |
| No `--force`, no `brew link`, only `install`/`tap`/`trust` | `plan.Validate`. The e2e tests also assert on every recorded invocation (`testkit.AssertSafeInvocations`). |
| No shell strings, and no inherited tap-trust bypass | `execrunner` (argv only; `HOMEBREW_NO_REQUIRE_TAP_TRUST*` removed) |
| Detection makes no network calls | `scan` env (`HOMEBREW_NO_AUTO_UPDATE=1`, `HOMEBREW_NO_ENV_HINTS=1`). `brew info --installed` is local. |
| Never install over files `facman` does not own | `conflicts` pre-flight, then `ownership.Classify` (`conflict`), then `planning` exclusion |
| Unknown means "do not install" | `planning` never plans a package with incomplete detection |
| Passwords never pass through `facman` | `tui` `tea.ExecProcess` hand-off, and headless inherited stdio |

## Testing

| Level | Where | What |
|---|---|---|
| Unit | beside the code | Table-driven, with fakes for ports. Domain and use cases at 97 to 100%. |
| End-to-end | `cmd/facman/*_test.go` | In-process through the real wiring, with injected `options` (R-3) and a fake `brew` built from `testdata/fakecmd`. Every JSON document is validated against the schema. Consent goes through a pseudo-terminal. Covers crash recovery, the TUI on a pty, and Linux-no-display and WSL fixtures. |
| Integration | `cmd/facman/integration_test.go` (`integration` tag) | Real Homebrew. `scan`, `plan` and `doctor` run read-only with a temp state directory, and skip without brew or as root. The GNU `hello` install through `testdata/catalogs/integration` runs only with `FACMAN_INTEGRATION_MUTATE=1`, which CI sets on macOS only. |
| Performance smoke | `cmd/facman/perf_test.go`, `adapter/tui` | `--version` well under its budget, scan of the full catalog on the fake machine in about 0.5 s, and the first TUI frame in 20 to 52 ms. |

`make cover` reports about 96% overall. CI runs `make cover`, `make integration`, `make build` and a `facman --version` smoke test on Linux and macOS (see [.github/README.md](../.github/README.md)).

## Dependencies

| Module | Used for |
|---|---|
| `github.com/spf13/cobra`, `github.com/spf13/pflag` | Command tree and flags (AD-3) |
| `charm.land/bubbletea/v2` v2.1.1, `charm.land/lipgloss/v2` v2.1.0, `github.com/charmbracelet/x/ansi` | TUI (no `bubbles`) |
| `golang.org/x/term` | Terminal detection for the TUI and the prompt |
| `modernc.org/sqlite` | State DB without cgo, so the binary stays static (`CGO_ENABLED=0`) |
| `go.yaml.in/yaml/v3` | Catalog YAML with line and column numbers |
| `github.com/creack/pty`, `github.com/santhosh-tekuri/jsonschema/v6` | Tests only: pseudo-terminals and schema validation |

Go 1.27 (the `go` directive in `go.mod`). Binaries are static for darwin and linux, amd64 and arm64 (`make build-all`, GoReleaser).

## Data formats

- Catalog and profile YAML, `apiVersion: facman/v1`: [data dictionary §6](../specs/261114-software-factory-manager/data-dictionary.md#6-catalog-and-profile-yaml-embedded-apiversion-facmanv1), and the user reference in [user-docs/configuration.md](../user-docs/configuration.md#catalog-directories).
- Report schema v1 (`--json`): [internal/adapter/report/schema/report.v1.json](../internal/adapter/report/schema/report.v1.json) and [data dictionary §7](../specs/261114-software-factory-manager/data-dictionary.md#7-report-schema-v1---json).
- State DB tables (`meta`, `packages`, `operations`, `detections`, `tap_trust`, and `services`, which is reserved for Increment 2): [data dictionary](../specs/261114-software-factory-manager/data-dictionary.md) and `internal/adapter/sqlite`.

Back to [docs](README.md).
