# Data Dictionary: software-factory-manager, Increment 1

**Feature:** software-factory-manager (`facman`)
**Date:** 2026-11-14
**Purpose:** The data structures Increment 1 introduces: domain entities and value objects, use case ports, persisted tables, file formats and the `--json` report. Go names are indicative; field sets are binding for this increment. Each section names the package that owns the type (see [architecture.md](architecture.md)).

## 1. Enumerations (domain)

| Type | Package | Values | Notes |
|---|---|---|---|
| `PlatformKey` | `domain/platform` | `darwin`, `linux`, `wsl` | Selects the catalog `install` block |
| `Kind` | `domain/catalog` | `cli`, `gui` | Drives WSL inheritance and display rules |
| `BackendID` | `domain/catalog` | `brew-formula`, `brew-cask`, `npm`, `report`, `host-report` | `npm` parses but has no adapter in Increment 1, so it yields `unavailable-here` |
| `VersionPolicy` | `domain/catalog` | `track`, `pinned`, `delegate` | Parsed and stored; behaviour deferred (FR-019) |
| `OwnershipState` | `domain/ownership` | `absent`, `foreign`, `managed`, `adopted`, `drifted`, `duplicate`, `conflict`, `unavailable-here` | Exactly one per package per scan. `suppressed` is reserved for Increment 2 (`-nop`) |
| `Provenance` | `domain/ownership` | `brew`, `brew-cask`, `native`, `npm`, `nvm`, `fnm`, `asdf`, `volta`, `script`, `app-bundle`, `os`, `unknown` | Where a detected install came from |
| `DetectorID` | `domain/ownership` | `path`, `brew`, `bundle`, `npm` | |
| `DetectionStatus` | `domain/ownership` | `ok`, `unknown` | `unknown` = detector timed out or failed; carries a reason |
| `OwnerClass` | `domain/plan` | `this-package`, `other-brew`, `npm`, `other-installer`, `orphan` | Conflict owner (FR-015) |
| `ActionKind` | `domain/plan` | `bootstrap-homebrew`, `tap`, `trust`, `install` | Increment 1 only; `upgrade`, `uninstall`, `adopt`, `forget`, `move-aside` arrive later |
| `ExclusionReason` | `domain/plan` | `conflict`, `dependency-excluded`, `unavailable-here`, `detection-incomplete`, `not-selected` | Why a wanted package is not in the plan |
| `OperationOp` | `domain/inventory` | `install`, `tap`, `trust`, `bootstrap` | Stored in `operations.op`. Later: `upgrade`, `uninstall`, `adopt`, `forget` |
| `OperationStatus` | `domain/inventory` | `in_progress`, `succeeded`, `failed`, `interrupted` | |
| `Severity` | `domain/bootstrap`* | `info`, `warning`, `fatal` | Bootstrap and doctor findings |
| `FailureKind` | `domain/failure` | `usage`, `validation`, `bootstrap`, `operation`, `drift` | Maps to exit codes 64, 3, 4, 1, 2 |

\* `BootstrapReport` and `Finding` live in `domain/platform` alongside the platform facts they describe (see §2.9), to keep the domain package count small.

## 2. Entities and value objects (domain)

### 2.1 `Version` (`domain/version`)

| Field | Type | Notes |
|---|---|---|
| `Raw` | string | As printed by the tool |
| `Parts` | []int | Numeric dot components; non-numeric suffixes kept in `Pre` |
| `Pre` | string | Pre-release suffix |

Functions: `Parse(string) (Version, error)`, `Compare(a, b) int`, `AtLeast(v, min) bool`. Unknown or unparsable versions never satisfy a minimum.

### 2.2 `Platform` (`domain/platform`)

| Field | Type | Notes |
|---|---|---|
| `OS` | string | `darwin` or `linux` |
| `Arch` | string | `amd64`, `arm64` |
| `WSL` | bool | |
| `Display` | bool | Always true on darwin |
| `Glibc` | `*version.Version` | nil on darwin or when unknown |
| `Distro` | `Distro{ID, IDLike []string, PkgManager}` | `PkgManager`: `apt`, `dnf`, `""` (unknown) |
| `UID` | int | |

Methods: `Key() PlatformKey` (wsl when `WSL`), `GlibcBelow(min) bool`.

### 2.3 `CatalogEntry` (Package) (`domain/catalog`)

| Field | Type | Notes |
|---|---|---|
| `ID` | string | Kebab-case, unique (e.g. `claude-code`) |
| `Display`, `Category` | string | |
| `Kind` | `Kind` | Required except for choice groups |
| `Platforms` | []`PlatformKey` | Empty = all |
| `Detect` | `DetectSpec` | |
| `Install` | map[`PlatformKey`][]`InstallStep` | Missing key = not offered on that platform; `wsl` inherits `linux` when `Kind == cli` |
| `VersionPolicy` | `VersionPolicy` | Default `track` |
| `Requires` | []`Requirement{Package string, MinVersion string}` | |
| `SatisfiesIf` | `*Satisfaction{MinVersion string}` | Any provenance at or above this counts as present (node) |
| `RejectIf` | `*Rejection{VersionLT string}` | Detected below this = not the wanted tool (awscli v1 → `foreign`, not satisfying) |
| `Choice` | []`ChoiceMember{ID string, Platforms []PlatformKey, HostOn []PlatformKey}` | Non-empty = choice group |
| `SatisfiedByAnyPresent` | bool | Choice groups |
| `SignInRequired` | bool | Drives the Summary next steps (FR-112): true for claude-code, claude-desktop, copilot-cli, github-copilot-app, antigravity-cli |
| `Service` | `*ServiceSpec` | Parsed and kept opaque in Increment 1 (Paperclip) |

`DetectSpec`: `Commands []string`, `BundleIDs []string`, `WingetID string`, `Version *VersionProbe{Cmd []string, Regex string}`, `Provenance map[Provenance]string` (path glob with `{brew_prefix}`, `{npm_prefix}`, `~`), `NPMPackage string`.

`InstallStep`: `Backend BackendID`, `ID string` (formula/cask/npm name, full tap path for tap items), `Tap string`, `Trust string` (only `item`), `Version string`, `Hint string`, `PostInstall []string` (parsed, unused in Increment 1). `Link: force` from the design is **not supported** (D-3).

### 2.4 `Catalog` (`domain/catalog`)

| Field | Type | Notes |
|---|---|---|
| `APIVersion` | string | `facman/v1` |
| `Entries` | []`CatalogEntry` | Ordered as in the file (the planner's tie-break order) |
| `Profiles` | map[string]`Profile` | |
| `Revision` | string | `embedded:<facman version>` or `dir:<abs path>@<sha256 of files>` → `packages.manifest_rev` |

`Validate() error` returns a `failure.Validation` listing every problem: duplicate IDs, unknown `requires`/choice members/profile packages, include cycles, requires cycles, missing `kind`, unknown backend, `trust` other than `item`.

### 2.5 `Profile` and `DesiredSet` (`domain/catalog`)

`Profile`: `Name`, `Description`, `Include []string`, `Packages []ProfilePackage{ID string, Optional bool, Version string}`, `Policy map[string]any` (parsed, unused).

`Resolve(catalog, profiles []string, with []string) (DesiredSet, error)`:

| `DesiredItem` field | Type | Notes |
|---|---|---|
| `ID` | string | |
| `Required` | bool | Required in any selected profile beats optional |
| `Selected` | bool | `Required`, or optional and named by `--with` |
| `Version` | string | Pin; two different pins across profiles → validation error naming both profiles |
| `FromProfiles` | []string | Shown on `i` ("which profile asked") |
| `ChoicePick` | string | Member chosen through `--with <member>` or the TUI |

### 2.6 `Detection` (`domain/ownership`)

| Field | Type | Notes |
|---|---|---|
| `PackageID` | string | |
| `Detector` | `DetectorID` | |
| `Status` | `DetectionStatus` | |
| `Reason` | string | For `unknown` |
| `Found` | bool | |
| `Path` | string | As found on PATH or bundle path |
| `ResolvedPath` | string | After `filepath.EvalSymlinks` |
| `Provenance` | `Provenance` | |
| `Version` | string | `""` = unknown |
| `BackendRef` | string | e.g. `brew-cask:claude-code` when brew reports it |
| `PathWinner` | bool | First match on PATH (for `duplicate`) |

### 2.7 `PackageStatus` (classification result) (`domain/ownership`)

| Field | Type | Notes |
|---|---|---|
| `PackageID` | string | |
| `State` | `OwnershipState` | |
| `Detections` | []`Detection` | |
| `Winner` | *`Detection` | For `duplicate` |
| `OSBundled` | bool | Displayed as "present (OS)" |
| `UnavailableReason`, `Hint` | string | For `unavailable-here` |
| `Conflict` | *`plan.Conflict` | For `conflict` |
| `Incomplete` | bool | A detector relevant to this package was `unknown` |
| `Record` | *`inventory.Record` | Inventory row, if any |

`Classify(entry, detections, record, conflict, availability) PackageStatus` precedence: `unavailable-here` → `conflict` (only if otherwise `absent`) → inventory-based (`managed`/`adopted` when detected through the recorded backend, else `drifted`) → `duplicate` (≥ 2 distinct provenances found, not counting the same file seen by two detectors) → `foreign` (found, no record) → `absent`.

### 2.8 Plan, Action, Command, Conflict, TapTrustStep, Consent (`domain/plan`)

`Command`:

| Field | Type | Notes |
|---|---|---|
| `Argv` | []string | Exact argv; `Argv[0]` is the program (`brew`, `/bin/bash`) |
| `EnvNames` | []string | Names of variables set for this command (values never logged) |
| `NeedsSudo` | bool | Cask `pkg`/`installer` artifact, or the Homebrew installer |
| `Unverified` | bool | Homebrew installer script |
| `Interactive` | bool | Terminal handed to the child (= `NeedsSudo`) |
| `Mutating` | bool | Always true for plan commands |

`Action`: `ID int` (order), `Kind ActionKind`, `PackageID string`, `Commands []Command`, `DependsOn []int`, `TapTrust *TapTrustStep`.

`TapTrustStep`: `Tap string` (`stablyai/orca`), `ItemKind` (`cask`|`formula`), `Item string` (`stablyai/orca/orca`), `PackageID string`.

`Plan`: `Profiles []string`, `Actions []Action` (topologically ordered), `Excluded []Exclusion{PackageID, Reason ExclusionReason, Detail string}`, `Warnings []Finding`, `CatalogRevision string`. Invariant check `plan.Validate()` rejects: any argv containing `--force` or `--overwrite`; a `brew trust` whose target is not a full item path; any command for a package whose state is not `absent`; `brew upgrade`/`uninstall`/`untap`/`untrust`.

`Conflict`: `PackageID`, `Path`, `Owner OwnerClass`, `OwnerDetail` (e.g. `brew formula gemini-cli`, `npm @anthropic-ai/claude-code`), `Evidence ConflictEvidence`, `FailingCommand []string`, `FixHint string`.

`ConflictEvidence`: `LsLine string`, `FileType string` (`file -b` output or Go-derived type), `IsSymlink bool`, `SymlinkTarget string`, `Dangling bool`, `Executable bool`, `Size int64`, `ModTime time.Time`.

`Consent` (value object): `By string` (OS user), `At time.Time`; `String()` = `<user>@<RFC 3339 UTC>` (FR-027). Later increments add `config:<path>@<sha>` and `setup:<path>@<sha>` constructors.

### 2.9 `BootstrapReport` and `Finding` (`domain/platform`)

| Field | Type | Notes |
|---|---|---|
| `Platform` | `Platform` | |
| `CLT` | `*bool` | macOS only; nil elsewhere |
| `LinuxPrereqsMissing` | []string | e.g. `gcc`, `file` |
| `PrereqCommand` | []string | Exact apt/dnf command to show |
| `Homebrew` | `HomebrewInfo{Found bool, Path, Prefix string, OnPATH bool, PrefixOwnedByOther bool, Version string}` | |
| `ShellEnv` | map[string]string | From `brew shellenv` (PATH, HOMEBREW_PREFIX, …) |
| `Node` | `NodeInfo{Found bool, Version string, Provenance Provenance, Satisfies bool, NPM bool}` | ≥ 24.11 satisfies |
| `Findings` | []`Finding{Code, Severity, Message, Fix string}` | e.g. `uid0` (fatal), `glibc-old` (warning), `brew-missing` (warning), `linux-prereqs` (fatal for apply), `brew-prefix-owner` (fatal for apply) |

### 2.10 Inventory types (`domain/inventory`)

`Record` (row of `packages`): `ID`, `State` (`managed`|`adopted`), `Backend BackendID`, `BackendRef`, `InstalledVersion`, `InstallPath`, `InstalledAt`, `InstalledBy`, `AdoptedAt *time.Time`, `ManifestRev`, `Scope` (`user` in Increment 1), `Bootstrap bool`.

`Operation` (row of `operations`): `ID int64`, `StartedAt`, `FinishedAt *time.Time`, `PackageID`, `Op OperationOp`, `Commands []plan.Command` (JSON), `ExitCode *int`, `Status OperationStatus`, `LogPath`, `ConsentBy`.

`DetectionSnapshot` (rows of `detections`): `ScannedAt`, plus `Detection` fields `PackageID`, `Found`, `Version`, `Provenance`, `Path`.

`TapTrustGrant` (row of `tap_trust`): `PackageID`, `Tap`, `Item`, `GrantedAt`, `OperationID`.

### 2.11 Failure (`domain/failure`)

`type Error struct { Kind FailureKind; Msg, Fix string; Err error }` with `ExitCode() int`. Constructors: `Usage`, `Validation`, `Bootstrap`, `Operation`, `Drift`. Precedence (spec, Edge cases): usage > bootstrap > validation > operation; `doctor` maps findings to `Drift`.

## 3. Use case ports (interfaces declared where consumed)

| Port | Declared in | Methods | Implemented by |
|---|---|---|---|
| `CommandRunner` | `usecase/execute`, `usecase/scan`, `usecase/bootstrap` (each declares the subset it needs) | `Run(ctx, Cmd) (Result, error)` where `Cmd{Argv, Env map[string]string, Dir, Interactive bool, Log io.Writer, Timeout}` and `Result{ExitCode, Stdout, Stderr}` | `adapter/execrunner` |
| `PlatformProbe` | `usecase/bootstrap` | `Probe(ctx) (platform.Platform, error)` | `adapter/platform` |
| `HomebrewLocator` | `usecase/bootstrap` | `Locate(ctx) (HomebrewInfo, error)`, `ShellEnv(ctx, brewPath) (map[string]string, error)` | `adapter/brew` |
| `Detector` | `usecase/scan` | `ID() DetectorID`, `Detect(ctx, []catalog.CatalogEntry, Env) ([]ownership.Detection, error)` | `adapter/detect` (path, bundle, npm), `adapter/brew` (brew) |
| `Inventory` | `usecase/scan`, `usecase/execute`, `usecase/status` | `Records(ctx)`, `UpsertRecord(ctx, Record)`, `StartOperation(ctx, Operation) (id, error)`, `FinishOperation(ctx, id, exitCode, status, logPath)`, `MarkInterrupted(ctx) ([]Operation, error)`, `SaveDetections(ctx, []DetectionSnapshot)`, `RecordTapTrust(ctx, TapTrustGrant)`, `TapTrustGrants(ctx)` | `adapter/sqlite` |
| `CatalogSource` | `usecase/planning`, `usecase/scan`, `usecase/doctor` | `Load(ctx) (catalog.Catalog, error)` | `adapter/catalogyaml` (embedded or dir) |
| `Backend` | `usecase/planning`, `usecase/conflicts` | `ID() BackendID`, `Available(ctx) bool`, `InstallCommands(ctx, entry, step) ([]plan.Command, error)`, `LinkPaths(ctx, entry, step) ([]string, error)`, `TapState(ctx, step) (tapped, trusted bool, err error)`, `Capabilities() Caps{NeedsSudo bool}` | `adapter/brew` (`brew-formula`, `brew-cask`) |
| `PathProbe` | `usecase/conflicts` | `Inspect(ctx, path) (*plan.ConflictEvidence, error)` (nil = nothing there), `Owner(ctx, path, entry) (OwnerClass, detail string, error)` | `adapter/fsprobe` |
| `HomebrewInstaller` | `usecase/execute` | `Download(ctx) (path string, err error)` (3 retries), `Command(path) plan.Command` | `adapter/installer` |
| `StateDir` | `usecase/execute`, `usecase/scan` | `LogPath(opID, pkgID, t) string`, `Lock(ctx) (Unlock func(), err error)`, `TryLock() (ok bool, holder int)`, `PruneLogs(now)`, `OpenLog(path) (io.WriteCloser, error)` (execute, I-28) | `adapter/statedir` |
| `Replanner` | `usecase/execute` | `AfterBootstrap(ctx, remaining plan.Plan) (Replan{Env, Present, Conflicts, Incomplete}, error)` (I-29) | `cmd/facman` wiring over bootstrap, scan and conflicts |
| `ConflictChecker` | `usecase/doctor` | `Check(ctx, entry, step) (*plan.Conflict, error)` (I-30) | `usecase/conflicts.Service` |
| `Clock`, `UserInfo` | `usecase/execute` | `Now() time.Time`; `Username() string` | `adapter/execrunner` (system) and test fakes |
| `Progress` | `usecase/execute` | `ActionStarted(Action)`, `CommandOutput(Action, line string)`, `ActionFinished(Action, Result)` | `adapter/cli` (text), `adapter/tui` (messages) |
| `Confirmer` | `usecase/execute` (headless path is driven by `adapter/cli`) | `Confirm(ctx, plan) (bool, error)` | `adapter/cli` (`[y/N]`), `adapter/tui` (Review `Enter`) |

## 4. Persisted schema (SQLite, migration 1)

```sql
CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);   -- schema_version=1, machine_id (random UUID), last_manifest_rev
CREATE TABLE packages(
  id TEXT PRIMARY KEY, state TEXT NOT NULL CHECK(state IN ('managed','adopted')),
  backend TEXT NOT NULL, backend_ref TEXT NOT NULL,
  installed_version TEXT, install_path TEXT,
  installed_at TEXT NOT NULL, installed_by TEXT NOT NULL, adopted_at TEXT,
  manifest_rev TEXT, scope TEXT NOT NULL DEFAULT 'user', bootstrap INTEGER NOT NULL DEFAULT 0);
CREATE TABLE operations(
  id INTEGER PRIMARY KEY AUTOINCREMENT, started_at TEXT NOT NULL, finished_at TEXT,
  package_id TEXT NOT NULL, op TEXT NOT NULL, commands_json TEXT NOT NULL,
  exit_code INTEGER, status TEXT NOT NULL, log_path TEXT, consent_by TEXT NOT NULL);
CREATE TABLE detections(
  scanned_at TEXT NOT NULL, package_id TEXT NOT NULL, found INTEGER NOT NULL,
  version TEXT, provenance TEXT, path TEXT);
CREATE TABLE services(
  package_id TEXT PRIMARY KEY, manager TEXT, unit_path TEXT, port INTEGER,
  exposure TEXT, last_health TEXT, last_backup_path TEXT);           -- created empty (Increment 2 writes it)
CREATE TABLE tap_trust(
  package_id TEXT NOT NULL, tap TEXT NOT NULL, item TEXT NOT NULL,
  granted_at TEXT NOT NULL, operation_id INTEGER REFERENCES operations(id));
CREATE INDEX operations_status ON operations(status);
CREATE INDEX detections_pkg ON detections(package_id, scanned_at);
```

- Times are RFC 3339 UTC text (readable with the system `sqlite3`, FR-062).
- Only `managed`/`adopted` rows exist in `packages`; other states are computed per scan, never stored. `drifted` is computed from a `packages` row plus detections.
- `detections` keeps the last 10 scans per package (older rows deleted at scan time) to bound size; `operations` is never pruned.
- The `homebrew` bootstrap install writes a `packages` row with `id = 'homebrew'`, `backend = 'script'`, `bootstrap = 1`.

## 5. Files and paths

| Item | macOS | Linux / WSL |
|---|---|---|
| State dir | `~/Library/Application Support/facman/` | `${XDG_STATE_HOME:-~/.local/state}/facman/` |
| DB | `<state dir>/state.db` | same |
| Lock | `<state dir>/facman.lock` | same |
| Logs | `<state dir>/logs/<YYYYMMDD-HHMMSS>-<operation id>-<package id>.log` | same |

## 6. Catalog and profile YAML (embedded, `apiVersion: facman/v1`)

Format per PRD Appendix F1/F2 and design §5, with these Increment 1 specifics:

- One file `catalog.yaml` with `apiVersion` and `packages:` (a map, but load order is preserved from the YAML node order).
- `profiles/<name>.yaml` with `apiVersion`, `profile`, `description`, `include`, `packages` (string or `{id: {optional: true, version: x}}`), `policy`.
- `--catalog <dir>` expects `<dir>/catalog.yaml` and `<dir>/profiles/*.yaml`.
- Added field `sign_in_required: true` on entries needing entitlement (FR-112).
- Unknown keys are a validation error with `file:line:col`, except under `service:` and `policy:` (kept for later increments).
- Choice members (`orbstack`, `colima`, `docker-desktop`) are full catalog entries; `colima` `requires` `docker` and `docker-compose` formula entries (hidden helpers, `display` only).

## 7. Report schema v1 (`--json`)

Embedded at `internal/adapter/report/schema/report.v1.json` (JSON Schema 2020-12). Top level:

| Field | Type | Present for |
|---|---|---|
| `schema` | const `"facman.report/v1"` | all |
| `command` | `scan`\|`plan`\|`apply`\|`status`\|`doctor` | all |
| `facman` | `{name, title, version}` | all |
| `generated_at` | RFC 3339 | all |
| `platform` | `{os, arch, wsl, display, glibc, distro}` | all |
| `bootstrap` | `{homebrew:{found,path,prefix,on_path}, clt, node:{found,version,provenance,satisfies}, findings:[{code,severity,message,fix}]}` | all |
| `catalog` | `{revision, profiles:[...]}` | all |
| `packages` | `[{id, display, state, os_bundled, provenance, path, version, detections:[...], winner, unavailable_reason, hint, conflict:{path, owner, owner_detail, evidence:{ls_line, file_type, symlink, symlink_target, dangling, executable, size, mod_time}, failing_command, fix_hint}, incomplete, selected, required}]` | scan, plan, apply, status |
| `plan` | `{actions:[{id, kind, package_id, commands:[{argv, env_names, needs_sudo, unverified}], depends_on}], excluded:[{package_id, reason, detail}]}` | plan, apply |
| `results` | `[{action_id, package_id, status, exit_code, log_path, operation_id}]` | apply |
| `consent_by` | string or null | apply (null on decline or dry run) |
| `findings` | `[{code, severity, package_id, message, fix}]` | doctor |
| `next_steps` | `[{package_id, message}]` (sign-in required, shellenv line) | apply |
| `exit_code` | int | all |

Optional fields are omitted, never `null`, except `consent_by`. `export` (Increment 5) adds `inventory` and `operations` under the same `schema` with a minor version bump.
