# PRD: software-factory-manager (Agentic Software Factory Manager, `facman`)

**Created:** 2026-11-14
**Jira:** N/A
**Status:** Draft

> **Structure.** The sections above the appendices are the PRD proper (problem, goals, 80+ numbered requirements, NFRs, acceptance criteria, risks, open questions). **Appendices A-K** hold the full command reference, flags, TUI keys, ownership states, the catalog, file formats, bootstrap checks, exit codes, inventory schema and paths, so the design's detail is not lost. Research notes, option analyses and source links stay in the design doc.
>
> **Sources.** Drafted from [INTENT.md](INTENT.md) and [docs/design/software-factory-installer-design.md](docs/design/software-factory-installer-design.md) (v0.3), with decisions from [docs/architecture-decision-record.md](docs/architecture-decision-record.md). Section references such as "§7a" point into the design doc. No interview was run, because the decisions were already made there. Anything the sources leave open is under **Open Questions** or marked `# TODO:`.

## Problem Statement

Lab machines arrive in every state. Some tools were installed through Homebrew, some by vendor scripts, some ship with the OS, and some are installed twice. Install docs drift, and blunt scripts overwrite things developers care about. Meanwhile each machine must carry the same toolset to take part in one or more software factories.

Three groups are affected:

- **Developers on lab machines** (mostly single-user Macs where the developer owns Homebrew) want the factory toolset without losing their own setup.
- **Build machines and CI runners** (mostly headless Linux) need the same toolset with no prompts, driven by a file checked into git.
- **Factory and lab owners** need to declare what a factory needs once (catalog, profiles, setup files) and get consistent, auditable machines.

`facman`, the **Agentic Software Factory Manager**, is a single Go binary with a terminal UI (TUI) and a headless CLI. It **detects first, proposes only what is missing, asks before acting, and manages only what it installed.**

## Goals

- **G1. Consistent toolset.** Any supported machine reaches the same declared toolset (catalog v1, see FR-040) through one tool, with no hand-run install docs.
- **G2. Safe by default.** Existing developer setups are never overwritten. Every change is shown, consented to, and recorded. The safest option is always the default.
- **G3. Unattended repeatability.** A reviewed, checked-in configuration list applies with no prompts on build machines (Linux CI first), is idempotent, and leaves an audit trail.
- **G4. Seeded tool configuration.** Hermes, Paperclip, Orca and T3 Code setup come from checked-in files, and every file format has a template, a validator, and a contract test so formats stay correct as tools change.

## Non-Goals

- Not an MDM. It does not replace Jamf or Intune, though they can distribute it.
- No central server. Fleet reporting is a JSON export in v1.
- No native Windows backend. Windows is served through WSL 2. Windows-host GUI apps are reported with WinGet hints (the `winget-host` backend is a later option).
- No managing tools `facman` did not install (unless adopted), and no auto-resolving duplicates.
- No secrets in configuration files. Files list env var names only.
- No multi-user Homebrew ownership. `facman` assumes single-user machines and stops if the brew prefix belongs to someone else.
- No configuration of tool authentication, licensing, MCP servers or managed settings in v1 (checked and reported only; `configure:` seeding covers the files listed in FR-090).
- No installing Gemini CLI (superseded by Antigravity CLI).

## Functional Requirements

Requirements are grouped by area. The "Phase" column in the table at the end of this section maps each group to the design's phased plan (§11).

### Identity and CLI

**FR-001:** The application is built and invoked as `facman` (entry point `cmd/facman`). The repo is `software-factory-manager`, and the product's full name, **Agentic Software Factory Manager**, appears in the binary's text output.
**FR-002:** `facman --version` prints a one-line version string that includes the full product name. `facman --about` prints the title, description, version and repository name.
**FR-003:** The binary is a single static Go executable with no runtime dependency, so it runs on a machine with nothing installed.
**FR-004:** The CLI provides headless commands with the same capability as the TUI: `scan`, `plan`, `apply`, `status`, `upgrade`, `uninstall`, `adopt`, `forget`, `restore`, `doctor`, `export`, `setup`, `service`, `config`, `template`, `validate`.
**FR-005:** The short flags `-cl`, `-hs`, `-ps`, `-os` and `-ts` are accepted as exact tokens (also `-x=<file>`) and rewritten to their long forms. `-c` is also the single-letter shorthand for the configuration list. A unit test covers the full flag set against collisions.
**FR-006:** A required flag value that is missing (for example `-cl` with no file) is a usage error (exit 64). There is no implicit default file.
**FR-007:** `--json` writes a machine-readable result using the same schema as `facman export`.

### Detection and ownership

**FR-010:** Detection runs first and independently of the backends. It checks PATH (resolving symlinks to find provenance), `brew info --json=v2 --installed`, app bundles by bundle ID (with a fallback scan of `/Applications` and `~/Applications`), and `npm ls -g --json`.
**FR-011:** Each package on each machine is in exactly one ownership state: `absent`, `foreign`, `managed`, `adopted`, `drifted`, `duplicate` or `conflict`.
**FR-012:** `facman` may upgrade, uninstall or repair only `managed` and `adopted` packages. For `foreign` packages it reports only, and offers **Adopt**. Uninstalling an `adopted` package needs an extra confirmation.
**FR-013:** `drifted` packages (recorded but not detected, or detected through a different backend) are flagged, with **Reinstall** and **Forget** offered.
**FR-014:** `duplicate` packages are reported with which install wins on PATH. `facman` never resolves duplicates automatically.
**FR-015:** Before planning, `facman` checks every path a brew formula or cask will link. If a file or symlink not owned by that package is there, the package is marked `conflict` and classified: owned by this package, another brew item, npm, another installer, or orphan.
**FR-016:** For a `conflict`, the Plan screen shows the evidence (`ls -l`, file type, symlink target and whether it dangles, executable bit, size, modification time, the owner found, and the failing command).
**FR-017:** The only offered fix for a conflict is **move aside, then install**, or removal through the owning tool (for example `npm uninstall -g …`). Files are moved to a timestamped backup directory, never deleted. `facman` never passes `--force` or `--overwrite` to brew. `facman restore <id>` puts a moved-aside file back.
**FR-018:** `facman doctor` runs the conflict and tap-trust scans for the whole catalog without planning an install.
**FR-019:** For self-updating apps, `facman` owns presence and removal, and delegates version to the app's updater unless the manifest sets `version_policy: pinned` and the backend supports pinning.
**FR-020:** Detection must be correct per platform: it never runs `/usr/bin/git` before confirming Command Line Tools exist (`xcode-select -p`), and it treats OS-bundled git, sqlite and jq as "present (OS)".

### Planning, consent and execution

**FR-025:** Every mutation is expressed as a list of concrete commands, shown before consent. The same list serves `--dry-run`.
**FR-026:** In the TUI, the flow is Scan → Plan → Review → Run → Summary. Plan is a table grouped by state: missing required packages are pre-checked, optional ones are not, and `foreign` ones can only be adopted. Review is the consent gate and shows every command, whether it needs sudo, whether a script is unverified, and dependency order.
**FR-027:** Consent is recorded in the inventory (`operations.consent_by`) with the OS user and timestamp. In unattended runs the consent is `config:<path>@<git-sha or file sha256>`.
**FR-028:** Packages declare `requires:` dependencies, and the planner orders them first (for example Node before Paperclip).
**FR-029:** Backends are pluggable behind one interface (detect, plan, apply, outdated, capabilities). v1 backends: `brew-formula`, `brew-cask`, `npm`.
**FR-030:** Script installs (when a backend uses them) are downloaded to a temp file, never piped from `curl` to a shell, and are verified against a manifest-pinned SHA-256 where the vendor publishes one. Unverified scripts show a warning, and lab policy can forbid them.
**FR-031:** Homebrew is never run as root. For steps that need sudo (some cask `.pkg` installers), the terminal is handed to the child process so `facman` never captures credentials.
**FR-032:** Every action is written to the operations log before and after it runs. A crash leaves an `in_progress` record that the next launch reconciles, so runs are idempotent and resumable.
**FR-033:** `facman` upgrades and uninstalls only what it owns. Uninstalling a package that `facman` tapped or trusted offers to untrust and untap it if nothing else uses it.

### Catalog, profiles and platforms

**FR-040:** The catalog (v1) contains: Claude Code, Claude desktop, GitHub Copilot CLI, GitHub Copilot app, Codex CLI, Antigravity CLI, Hermes Agent, Orca, T3 Code, Paperclip, git, GitHub CLI, tmux, VS Code, iTerm2, sqlite, Node.js (`node@24`), uv, jq, ripgrep, AWS CLI v2, and the optional AWS Session Manager plugin, Google Cloud CLI, Azure CLI, and a container-runtime choice group (Docker Desktop, OrbStack or Colima, default off).
**FR-041:** Codex CLI, uv, jq and ripgrep are on by default. All items marked optional are off by default.
**FR-042:** Catalog and profiles are YAML validated against a published JSON Schema. Profiles compose (`include:`) and mark packages `optional`. Choice groups must name their pick.
**FR-043:** Every catalog entry declares a `kind` (`cli` or `gui`) and a per-platform install block (`darwin`, `linux`, `wsl`). A missing block means "not offered here". On WSL, CLIs inherit the Linux block, and GUI apps default to a host target.
**FR-044:** Platform detection covers OS, architecture, WSL, display presence and glibc version. Items the profile wants but this platform cannot install through any configured backend get the `unavailable-here` state: shown greyed with the reason and a hint, and they never block the run.
**FR-045:** On WSL, Windows-host GUI apps are report-only in v1 (present, foreign or missing, with a `winget` hint).
**FR-046:** The catalog and profiles live in one git repo owned by the platform team. `facman` pulls a tagged release, verifying the signed tag or a checksum. The manifest may declare `min_facman_version`.
**FR-047:** Third-party taps (today only `stablyai/orca`) declare `tap:` and `trust: item`. The plan includes the per-item `brew trust` step as its own reviewed step. `facman` never grants whole-tap trust and never sets the deprecated `HOMEBREW_NO_REQUIRE_TAP_TRUST` variables.
**FR-048:** Foreign untrusted taps are reported as info (naming the installed items brew now skips and the exact `brew trust` command). `facman` never trusts them on the developer's behalf.

### Startup bootstrap

**FR-050:** On every launch, before Scan, `facman` runs a bootstrap check: platform, Xcode CLT (macOS), Linux prerequisites, Homebrew, shell environment, and Node/npm.
**FR-051:** In the TUI, a missing Homebrew prompts with the full command shown. With `-cl`, it is installed automatically unless `bootstrap.homebrew: never`. Homebrew's official installer is downloaded to a temp file and run non-interactively as the invoking user.
**FR-052:** Node/npm is checked always, but installed or prompted for only when a selected package uses the npm backend. `node@24` is linked or put on PATH. A version-manager Node (nvm, fnm, asdf, Volta) at or above 24.11 satisfies the check as `foreign`.
**FR-053:** `facman` evaluates `brew shellenv` for itself and every child process. Persisting it to the login profile is a marked, reviewed block, a prompt in the TUI and governed by `bootstrap.persist_shellenv` under `-cl`. The block is recorded so exactly those lines can be updated or removed.
**FR-054:** With `-cl`, `facman` first checks `sudo -n true`. If it fails, it stops at once with exit code 4 and explains how to fix the image. It never hangs on a prompt.
**FR-055:** Homebrew or Node installed by `facman` is recorded as `managed` with a `bootstrap: true` flag. Homebrew is never uninstalled automatically. A pre-existing Homebrew is `foreign` and simply used.
**FR-056:** On Linux, the bootstrap installs distro prerequisites (apt or dnf) through a reviewed step, detects the distro from `/etc/os-release`, and exits with code 4 on an unknown distro. It detects UID 0 and exits with code 4 with a clear fix (Homebrew refuses to run as root). It looks for Homebrew at the known prefixes even when `brew` is not on PATH, and warns in `--dry-run` when glibc is older than 2.39.

### Inventory

**FR-060:** State is an embedded SQLite database (pure-Go driver, no cgo) at the platform's application-support path, holding `packages`, an append-only `operations` audit log, `detections` snapshots, `services`, and `meta`.
**FR-061:** Every launch reconciles the inventory (intent and ownership) with what package managers report (what is on disk). Disagreements show up as `drifted`.
**FR-062:** The developer can inspect the inventory with the `sqlite3` already on the machine, and `facman` does not depend on the catalog's `sqlite` package for its own state.

### Unattended runs: configuration lists

**FR-070:** A configuration list is a single YAML file (`kind: ConfigurationList`) recording the answers a developer would give in the TUI: packages, choice-group picks, install options, bootstrap settings, and policy.
**FR-071:** `facman apply -cl <file>` never opens the TUI and never reads stdin. Anything that would need a person is a hard failure with a clear message, never a hang.
**FR-072:** The checked-in file is the consent. Risky option values (for example Paperclip `exposure: remote`) are used only when written in the file. The safe default applies otherwise, and `--dry-run` prints it.
**FR-073:** Policy knobs: `policy.foreign` (`skip`, `adopt`, `fail`; default `skip`), `policy.unavailable_here` (`warn`, `fail`), `policy.allow_sudo`, `policy.conflicts` (`fail`, `move_aside`; default `fail`), `policy.tap_trust` (item-level), `policy.allow_whole_tap_trust` (default false), `policy.config_drift` (`keep`, `overwrite`, `fail`; default `keep`), and `policy.format_mismatch` (`warn`, `fail`).
**FR-074:** `--dry-run` prints the plan and changes nothing. `--check` exits 2 if the machine differs from the list.
**FR-075:** Exit codes: `0` success or already compliant, `1` an operation failed, `2` drift found (with `--check`), `3` validation error, `4` bootstrap failed, `64` usage error.
**FR-076:** Running the same list twice changes nothing the second time.
**FR-077:** In the TUI, `s` on Plan or Review saves the current answers as a configuration list. `facman config init|export|validate` manage lists headlessly.
**FR-078:** The Linux headless path is designed and tested first. GUI packages resolve to `unavailable-here` and are skipped with a warning when there is no display, so one list serves a developer Mac and a headless runner. A service that cannot run in a container without systemd is reported `unavailable-here` unless the list marks it `required: true`.

**FR-079:** `facman plan` and `facman apply` accept `--profile <name>` (a machine selects one or more profiles, and the union is the desired state), `--with <id>` (include an optional package in headless mode), `--set <key>=<value>` (answer a package option headlessly, for example `paperclip.exposure=remote`), and `--catalog <path|git-url>` (use a checked-in catalog, such as `./catalog`, instead of the default).
**FR-079a:** With `bootstrap.prune: true`, a configuration-list apply uninstalls `managed` items that are no longer listed. Default is `false`. Homebrew is never pruned (FR-055).
**FR-079b:** A configuration list pins its catalog by git tag and `sha256` (`catalog.source`, `catalog.ref`, `catalog.sha256`) and declares `platforms:`. Apply refuses on a platform not listed. (Whether to embed a catalog snapshot instead is Open Question 10.)
**FR-079c:** `facman config init --profile <name> -o <file>` writes a starting list from a profile's defaults. `facman config export -o <file>` captures what the machine currently manages. `facman config validate -cl <file>` checks a list against the schema and the pinned catalog (unknown IDs, a missing choice pick, an option value the package does not offer, platform mismatch) and is intended as a pre-merge CI check.

### Managed service: Paperclip

**FR-080:** Paperclip runs as a service `facman` manages: install the npm package, run `paperclipai onboard --yes` as the developer (never root), then register the service as a launchd LaunchAgent (macOS) or a systemd user unit (Linux). The unit or plist path is recorded for later removal.
**FR-081:** After every install, upgrade or restart, `facman` health-checks `http://127.0.0.1:3100` until it responds or times out. The Manage tab shows each service as running, stopped or unhealthy, with start, stop, restart and logs actions.
**FR-082:** Network exposure is an install choice. "This machine only" (localhost) is the default. Remote access enables Paperclip's login mode, collects the base URL and allowed hostnames, and is never picked by `facman` itself. Headless runs need an explicit `--set paperclip.exposure=remote` or the value in the list. The choice is stored (`services.exposure`) and can be changed later with the same consent step.
**FR-083:** Before upgrading Paperclip, `facman` backs up `secrets/master.key` and the embedded database directory, stops the service, upgrades, starts it and health-checks it. It rolls back to the previous version and backup if the health check fails.
**FR-084:** Uninstalling Paperclip stops and unregisters the service and removes the package. The `~/.paperclip` data directory is kept unless the developer separately confirms its deletion.
**FR-085:** On WSL, `facman` checks that systemd is enabled and shows the fix if not.
**FR-086:** `--no-paperclip` / `-nop` is a plain switch (no value; `-nop=<x>` is exit 64) that removes Paperclip from consideration for the run: it is dropped from the plan in the TUI, `plan`, `apply --profile` and `apply -cl`, along with its service step, exposure question, setup and Paperclip-only requirements (Node is not installed on its account). With `-cl` it overrides a listed Paperclip with a warning and records `suppressed=paperclip` in the consent record. It conflicts with `-ps` (exit 64). It never installs, upgrades, reconfigures, stops or removes an existing Paperclip, and scan and status still report it.
**FR-087:** The `service:` block is generic so later services (for example local MCP servers) can reuse the lifecycle.

### Seeding tool configuration

**FR-090:** Per-tool setup is supplied from files by `--hermes-setup`/`-hs`, `--paperclip-setup`/`-ps`, `--orca-setup`/`-os` and `--t3-setup`/`-ts`, each requiring a filename. A setup spec is a YAML file (`kind: ToolSetup`) holding that tool's `configure:` steps. `-ps` also accepts a Paperclip company package (folder, `.zip` or `github.com/...` URL).
**FR-091:** `configure:` step types are `files`, `env_file`, `command` and `json_merge`. `json_merge` deep-merges a JSON fragment and records the merged keys so only what `facman` added can be removed later. Relative paths resolve against the spec file's own directory. A package entry may reference a spec with `setup: <path>`.
**FR-092:** A setup flag runs stand-alone against an installed tool (any ownership state), or alongside `-cl`, where it replaces that tool's `configure:` section for the run. Several setup flags may be combined, and each may appear once. Order: install, then service start, then setups, then health checks. If the tool is not installed, the TUI offers to install it, and a non-interactive run fails with exit code 3 unless the same run installs it.
**FR-093:** A setup flag means the file is the consent (non-interactive), recorded as `consent_by=setup:<path>@<sha>`. `--review` opens the TUI Review screen first.
**FR-094:** Secrets never live in git. Specs declare only env var names. If a value is missing, the run fails with exit code 3 under `-cl`, or prompts in the TUI.
**FR-095:** Ownership extends to files. `facman` records a hash of every file it writes and does not overwrite a file the developer edited, flagging it `drifted` with a diff in the TUI. `policy.config_drift` decides under `-cl`. `command` steps marked `once: true` are recorded and skipped afterwards, and Paperclip imports use `--dry-run` under `facman apply --dry-run`.

### Formats, templates and validation

**FR-100:** `facman template` generates annotated starter files for `config-list`, `hermes`, `paperclip`, `orca` and `t3`, with a `# yaml-language-server: $schema=…` header, generated from the same schemas the validator uses. `--minimal` omits comments, and `--from-machine` pre-fills from the current machine, copying secret names only.
**FR-101:** `facman validate` checks, in order: syntax (with `file:line:col`), schema, semantics (catalog IDs, choice picks, option values, referenced paths, declared env var names, and no committed secret-looking values, which is an error), version fit (a warning, or an error under `--strict`), and optionally `--deep` (the tool's own checker). It supports `--json` and `--format github`.
**FR-102:** `apply -cl` and every `facman setup -xs` validate first and refuse to run on errors. The TUI Review screen shows validation results, and a Files tab lists the repo's configuration and setup files with their status and `e` (edit), `t` (template), `d` (diff) actions.
**FR-103:** **Contract rule:** no `facman` release and no catalog version bump ships unless every format contract passes against the exact tool versions it pins. Pinned JSON Schemas and golden fixtures live in the repo, and contract tests run in CI on every pull request that touches the catalog, schemas, templates or code, and nightly against each tool's latest release. A pinned-version failure blocks the merge. A latest-release failure opens an issue and does not block.
**FR-104:** At apply time, if an installed tool's version is outside its schema's range, `facman` warns in the TUI, and fails under `-cl` unless `policy.format_mismatch: warn`.

### Fleet, distribution and self-update

**FR-110:** `facman export --json` writes a machine report (detections, inventory, manifest revision) suitable for an MDM extension attribute or a nightly job.
**FR-111:** `facman` supports `HTTPS_PROXY`, custom CA passthrough, and mirror overrides (an internal npm registry or Homebrew bottle mirror) in the manifest.
**FR-112:** The Summary screen shows "installed, sign-in required" next steps for tools that need entitlement (Copilot, Claude, Antigravity). Claude Code managed-settings and Copilot policies are checked and reported, not set.
**FR-113:** `facman` is distributed as a signed and notarized binary on GitHub Releases, a Homebrew tap, and an MDM package for lab images, and it updates itself from signed releases or through its tap.

### Phasing (from design §11)

| Phase | Requirement groups |
|---|---|
| 0. Spike | FR-010 detectors only (`facman scan --json`) on 3-5 real lab Macs |
| 1. MVP | Identity (FR-001-003), detection, bootstrap (FR-050-055), brew-formula and brew-cask backends, catalog and profiles, inventory, TUI flow (FR-026), install only |
| 2. Manage | Upgrade, uninstall, adopt, forget, drift; npm backend; service manager (FR-080-087, including `-nop`); headless parity (FR-004) |
| 2b. Config lists + Linux CI | FR-070-078, Linux bootstrap (FR-056), exit codes |
| 2c. Formats | FR-100-104 |
| 3. Fleet | FR-046, FR-110, MDM packaging |
| 4. Linux desktop + WSL | FR-043-045 polish, host detection |
| 4b. Host GUI apps | `winget-host`, `native-pkg` backends |
| 5. Configure | FR-090-095 |

## Non-Functional Requirements

- **Performance:** # TODO: no targets are set in the sources. Propose targets for cold start, Scan on a typical machine, and `apply -cl --check` on an already-compliant machine. The design notes only that the bootstrap should take seconds when Homebrew is already present (GitHub Actions Ubuntu runners), and that Scan runs detectors in parallel.
- **Reliability:** Idempotent and resumable runs (FR-032, FR-076). A crash mid-run is reconciled on next launch. Upgrades of stateful services back up first and roll back on a failed health check (FR-083). `# TODO:` set an explicit success-rate or flake budget for unattended `apply -cl` on CI.
- **Security:**
  - Never run Homebrew as root, never capture sudo credentials, never pipe `curl` to a shell (FR-030, FR-031).
  - No secrets in any committed file, enforced by `facman validate` as an error (FR-101).
  - Risky options need explicit selection. Paperclip is localhost by default (FR-072, FR-082). Trust is item-level only (FR-047).
  - Releases are signed and notarized, and manifest repo pulls verify a signed tag or checksum (FR-046, FR-113).
- **Observability:** Append-only audit log with consent, commands, exit codes and log paths (FR-060). `--json` output on every headless command (FR-007). Deterministic exit codes (FR-075). GitHub Actions annotations from `validate --format github`. `# TODO:` define log retention and a log-location convention.
- **Portability:** macOS (primary), Linux (Tier 1 Homebrew: glibc 2.39 or newer, x86_64 and ARM64), and WSL 2. Single static binary, no cgo (FR-003, FR-060). State files live on the Linux filesystem on WSL, never under `/mnt/c`.
- **Usability:** The TUI shows exact commands and evidence before any change. Errors name the cause and the fix (for example exit 4 messages). Every format gets a template, schema-driven editor autocomplete, and line-numbered validation errors.
- **Maintainability:** Clean Architecture (domain, usecase, adapter), TDD, `make check` and CI green on every push (see [AGENTS.md](AGENTS.md)). Docs and READMEs are updated in the same change as the code.
- **Compatibility and drift control:** Pinned tool versions in the catalog, each tied to a schema version, with contract tests (FR-103). `# TODO:` Orca and T3 Code publish no formal schemas, so their schemas are `vendor-unofficial`. Define how much confidence these contract tests must give.

## Acceptance Criteria

- [ ] `facman --version` and `facman --about` print the full name "Agentic Software Factory Manager", and `make build` produces a single `bin/facman` binary with no runtime dependency (FR-001-003).
- [ ] On a lab Mac with some catalog tools already installed by the developer, `facman scan` marks them `foreign`, and a full TUI run proposes only the missing ones, changes nothing without consent, and leaves the foreign tools untouched (FR-011, FR-012, FR-026).
- [ ] A package with a stray file at its link path (for example a stale `/opt/homebrew/bin/claude`) is reported as `conflict` with evidence, and is installed only after an explicit move-aside that `facman restore` can undo. `facman` never uses `--force` (FR-015-017).
- [ ] `facman apply -cl <file>` on a clean Ubuntu 24.04 runner with passwordless sudo installs the listed packages with no prompt and no stdin read, exits 0, and a second run changes nothing (FR-071, FR-076).
- [ ] The same run fails with exit code 4 and a fix message when `sudo -n true` fails or UID is 0, and never hangs (FR-054, FR-056).
- [ ] `--dry-run` prints every command and changes nothing, and `--check` exits 2 when the machine differs from the list (FR-025, FR-074).
- [ ] Every operation appears in the audit log with `consent_by` set to the OS user or to `config:<path>@<sha>` (FR-027).
- [ ] `facman` upgrades and uninstalls only `managed` and `adopted` packages, and an adopted package's uninstall requires an extra confirmation (FR-012, FR-033).
- [ ] With Orca in a plan, Review shows a per-item `brew trust --cask stablyai/orca/orca` step, and `facman` never grants whole-tap trust unless `policy.allow_whole_tap_trust: true` (FR-047).
- [ ] Paperclip installs as a user service on localhost, passes its health check, and remote exposure happens only when explicitly chosen. A failed upgrade health check rolls back to the previous version and backup (FR-080-083).
- [ ] `--no-paperclip` / `-nop` leaves Paperclip (and Node, when Paperclip is the only npm user) out of the plan in the TUI, `plan` and `apply`. With `-cl` it warns about a listed Paperclip, with `-ps` it exits 64, and an existing Paperclip service is untouched (FR-086).
- [ ] `-hs`, `-ps`, `-os` and `-ts` each configure their tool from a spec file, work stand-alone and with `-cl`, never overwrite a developer-edited file, and fail with exit 3 when a required env var name has no value (FR-090-095).
- [ ] `facman template` outputs validate cleanly, and `facman validate` rejects a file containing a secret-looking value with a line-numbered error and exit 3 (FR-100, FR-101).
- [ ] CI contract tests pass against the pinned tool versions, and a release is blocked if any contract fails (FR-103).
- [ ] On a headless Linux runner, GUI packages show as `unavailable-here` and are skipped with a warning (`policy.unavailable_here: warn`), and on WSL, Windows-host GUI apps are reported with a `winget` hint and never installed (FR-044, FR-045, FR-078).
- [ ] `facman export --json` produces a report with detections, inventory and manifest revision that validates against the published schema (FR-110).

## Dependencies and Risks

| Item | Type | Notes |
|---|---|---|
| Homebrew 6+ (macOS, Linux, WSL) | Dependency | Primary backend. Requires tap trust for third-party taps, glibc 2.39 or newer on Linux, and a non-root user. |
| Node.js 24.11 or newer, npm, corepack/pnpm | Dependency | Needed only for Paperclip today. Provided by `node@24` through brew or a version manager. |
| Vendor tools (Claude Code, Copilot, Codex, Antigravity, Hermes, Orca, T3 Code, Paperclip, cloud CLIs) | Dependency | Install paths and formats are owned by vendors and can change. Pinned versions plus nightly contract tests are the mitigation. |
| Homebrew taps and casks (`stablyai/orca`) | Dependency | Bare `orca` is an unrelated Plotly cask, so the full tap path is required. |
| GitHub Actions, GoReleaser, golangci-lint | Dependency | CI and release pipeline (see [AGENTS.md](AGENTS.md)). Signing, notarization and the Homebrew tap are not yet in place. |
| Apple Developer signing and notarization | Dependency | Needed for the signed macOS release (FR-113). # TODO: owner and timeline. |
| Platform/lab team owning the catalog repo | Dependency | Needed for tagged, reviewed catalog releases (FR-046). |
| Vendor format drift (Hermes, T3 Code are most likely) | Risk | Contract tests (nightly against latest) are the early warning. Orca and T3 have no official schemas, so coverage is weaker. |
| Paperclip is a server with embedded Postgres and an encryption key | Risk | Data loss on bad upgrade. Mitigated by backup, health check and rollback (FR-083), and `~/.paperclip` is kept on uninstall. |
| Paperclip as a systemd user service in CI containers | Risk | Needs a user session (`loginctl enable-linger`). Reported `unavailable-here` unless marked `required`. |
| Linux bootstrap needs sudo and a distro package manager | Risk | Fails with exit 4 and a fix rather than guessing. Unknown distros are rejected. |
| Homebrew refuses to run as root, and CI containers often do | Risk | Exit 4 with guidance. `facman` does not create users. |
| Old glibc on CI images | Risk | Bottles are unavailable, so Homebrew builds from source and is slow. Warn in `--dry-run`. |
| Existing tool installs vary widely (npm-global launchers, native installers, version managers) | Risk | Conflict and duplicate detection. Never overwrite or auto-resolve. |
| Self-updating apps fight version pinning | Risk | `facman` owns presence and removal only unless pinned where supported (FR-019). |
| Licensing and sign-in gates (Copilot seat and policies, Claude plans, Google account) | Risk | Installs succeed but tools do not work without entitlement. The Summary shows next steps (FR-112). |
| Homebrew trust model changes (tap trust is new in Homebrew 6) | Risk | Stay on item-level trust and track Homebrew releases. |
| Multi-user machines | Risk | Out of scope. `facman` stops if the brew prefix is owned by someone else. |

## Open Questions

Carried from design §12 (unanswered) and §7a:

1. **Identities (Q2).** Confirm: hermes = Nous Research Hermes Agent (CLI, desktop app, or both)? paperclip = paperclipai/paperclip? orca = Stably AI Orca? "T3 desktop" = T3 Code, not T3 Chat?
2. **Minimum versions (Q4).** Are Apple's bundled git and sqlite acceptable, or must factories require minimum versions, which would mean installing brew copies alongside?
3. **Node managers (Q5).** If a developer uses nvm, fnm, asdf or Volta, install npm packages into their Node, or install a brew `node` that `facman` owns?
4. **Claude Code channel (Q7).** Is the brew `claude-code` cask (stable, upgraded by `facman`) the standard, or native auto-update with managed-settings version floors?
5. **Adoption default (Q8).** For already-brew-installed tools like `gh`, offer adoption interactively (current design), or auto-adopt in lab policy mode?
6. **Multiple factories (Q9).** Can one machine belong to several factories at once (profile union), and can two factories conflict by pinning different versions of the same tool?
7. **Distribution (Q10).** Is Jamf or another MDM available to push `facman`, or is it self-serve `brew install` or curl?
8. **WSL host apps (Q11).** Is report-only acceptable for Windows-host GUI apps in v1, or should `facman` install them through WinGet from the start (a second consent scope, possibly admin rights)?
9. **Linux desktops (Q12).** Are there Linux desktop machines in the lab? If so, VS Code and Claude desktop need an apt/dnf or Flatpak backend.
10. **Config lists and the catalog (Q13).** Pin the catalog by git tag (current design) or embed a snapshot of the entries a list uses so it is self-contained?
11. **Suppression scope (Q14).** Should `--no-paperclip` also have a persistent setting and generalize to `--no-<package>` / `--skip <id>`? Confirm the chosen defaults: with `-cl` it overrides a listed Paperclip with a warning, and an existing Paperclip is left untouched.
12. **Non-functional targets.** Performance, reliability and log-retention targets are `# TODO:` above and need owners and numbers.
13. **Release readiness.** Who owns Apple signing and notarization and the Homebrew tap, and by when? CI and release workflows exist but those parts do not.

---

# Appendices: full specification

The requirements above say *what* must be true. These appendices record the complete command structure, screens, file formats, catalog and defaults, so nothing in the design doc is lost in this PRD. Where the design leaves a detail open, it is marked `# TODO:`. Rationale and research stay in the [design doc](docs/design/software-factory-installer-design.md); section numbers (§) point there.

## Appendix A: Command reference

`facman` with no command opens the TUI. All other commands are headless. Global behaviour: `--json` (machine-readable output, same schema as `export`), `--dry-run` (print the plan, change nothing), `--version`, `--about`, `--help`/`-h`.

| Command | Purpose | Arguments and flags | Mutates? | Source |
|---|---|---|---|---|
| `facman` | Open the TUI: Scan → Plan → Review → Run → Summary, plus Manage and Files tabs | none | Yes, after consent | §7 |
| `facman scan` | Detect only: present, missing, managed, with provenance | `--json` | No | §7, §13 |
| `facman plan` | Show what would be installed | `--profile <name>`, `--with <id>`, `--set <k=v>`, `--catalog <path>`, `-nop`/`--no-paperclip`, `--dry-run`, `--json` | No | §7, §13 |
| `facman apply` | Install the plan. Interactive consent by default | `--profile <name>`, `--with <id>`, `--set <k=v>`, `--catalog <path>`, `-cl`/`--configuration-list <file>` (also `-c`), `-hs`/`-ps`/`-os`/`-ts <file>`, `-nop`/`--no-paperclip`, `--review`, `--dry-run`, `--check`, `--json` | Yes | §7a, §13 |
| `facman status` | Managed, adopted and foreign items, and drift | `--json` | No | §13 |
| `facman upgrade [<id>…]` | Upgrade managed and adopted items only | optional package IDs | Yes | §13 |
| `facman uninstall <id>` | Remove a managed or adopted item, with confirmation | package ID. Extra confirmation for adopted. Keeps `~/.paperclip` unless separately confirmed | Yes | §13, §6a |
| `facman adopt <id>` | Take over a pre-existing (foreign) tool | package ID | Records ownership | §3, §13 |
| `facman forget <id>` | Drop a drifted record from the inventory | package ID. `# TODO:` the design names Forget as an action (Manage tab, operations log) but does not list a `forget` CLI command, so confirm it exists | Inventory only | §3, §7, §6 |
| `facman restore <id>` | Put a moved-aside conflicting file back | package ID | Yes | §3a |
| `facman doctor` | Scan the whole catalog for conflicts and untrusted taps, without planning | `# TODO:` flags (at least `--json`) | No | §3a, §3b |
| `facman setup` | Apply a tool's setup from a file to an installed tool | `-hs`/`-ps`/`-os`/`-ts <file>`, each at most once, combinable. `-ps` also accepts a Paperclip company package (folder, `.zip`, `github.com/...` URL). `--review` | Yes | §7a, §13 |
| `facman service status\|start\|stop\|logs paperclip` | Manage the Paperclip service. `restart` and `Change exposure` are also available in the TUI Manage tab | action, service name | Yes (start/stop) | §6a, §13 |
| `facman export` | Machine report: detections, inventory, manifest revision | `--json` (for example `> report.json`) | No | §9, §13 |
| `facman config init` | Write a starting configuration list from a profile | `--profile <name>`, `-o <file>` | Writes a file | §7a |
| `facman config export` | Capture what this machine currently manages as a list | `-o <file>` | Writes a file | §7a |
| `facman config validate` | Check a list against the schema and the pinned catalog | `-cl <file>` | No | §7a |
| `facman template <kind>` | Write an annotated starter file. Kinds: `config-list`, `hermes`, `paperclip`, `orca`, `t3` | `-o <file>`, `--profile <name>` (config-list), `--minimal`, `--from-machine` | Writes files (setup kinds also write `assets/`, `company/` or `settings.fragment.json`) | §7b |
| `facman validate <path…>` | Check facman and tool-native files | `--deep`, `--strict`, `--format github`, `--json`. Accepts files and directories | No | §7b |

**Usage examples** (from the design):

```bash
facman                                                    # TUI
facman scan --json
facman plan --profile agentic-factory
facman apply --profile agentic-factory
facman apply --profile agentic-factory --no-paperclip     # short: -nop
facman apply -cl configs/build-macos.facman.yaml          # unattended
facman apply -cl build.facman.yaml --dry-run              # print the plan, change nothing
facman apply -cl build.facman.yaml --check                # CI gate: exit 2 if the machine differs
facman apply -cl configs/build-linux.facman.yaml -ps setup/paperclip/ci.setup.yaml   # override one tool's setup
facman setup -hs setup/hermes/team.setup.yaml
facman config init --profile agentic-factory -o configs/build-linux.facman.yaml
facman template hermes -o setup/hermes/team.setup.yaml
facman validate configs/ setup/ --strict --format github
facman export --json > report.json
```

Run from source: `go run ./cmd/facman …`, `go install github.com/stainedhead/software-factory-manager/cmd/facman@<tag>`, or `make build` (writes `./bin/facman`; design §13 also lists a `make build-all` for darwin/linux amd64+arm64, `# TODO:` not yet in the Makefile).

## Appendix B: Flags and short-flag rules

| Long | Short | Value | Applies to | Meaning |
|---|---|---|---|---|
| `--configuration-list` | `-cl` (also `-c`) | file (required) | `apply`, `config validate` | Apply or check a configuration list. The file is the consent |
| `--hermes-setup` | `-hs` | file (required) | `apply`, `setup` | Hermes setup spec |
| `--paperclip-setup` | `-ps` | file or company package (required) | `apply`, `setup` | Paperclip setup spec |
| `--orca-setup` | `-os` | file (required) | `apply`, `setup` | Orca setup spec |
| `--t3-setup` | `-ts` | file (required) | `apply`, `setup` | T3 Code setup spec (settings merge only) |
| `--no-paperclip` | `-nop` | none | `plan`, `apply`, TUI | Suppress Paperclip from consideration for this run |
| `--profile` | | name | `plan`, `apply`, `config init`, `template` | Select a profile. A machine may select several and the union is the desired state (`# TODO:` confirm the flag repeats) |
| `--with` | | package ID | `plan`, `apply` | Include an optional package headlessly |
| `--set` | | `key=value` | `plan`, `apply` | Answer a package option, for example `paperclip.exposure=remote` |
| `--catalog` | | path or git URL | `plan`, `apply` | Use this catalog instead of the default |
| `--review` | | none | `apply`, `setup` | Open the TUI Review screen before applying a file-consented run |
| `--dry-run` | | none | `plan`, `apply` | Print the plan and any validation, change nothing. Shows which setup source each tool used, and what `-nop` suppressed |
| `--check` | | none | `apply -cl` | Exit 2 if the machine differs from the list |
| `--json` | | none | all headless | Machine-readable result |
| `--output` | `-o` | file | `config init/export`, `template` | Output file |
| `--minimal`, `--from-machine` | | none | `template` | No comments, or pre-fill from this machine (secret names only) |
| `--deep`, `--strict`, `--format github` | | none/value | `validate` | Run the tool's own checker, escalate version-fit warnings, emit Actions annotations |
| `--version`, `--about`, `--help` | `-h` | none | root | Version line, application info, help |

Rules (§7a, FR-005, FR-006, FR-086):
- `-cl`, `-hs`, `-ps`, `-os`, `-ts` and `-nop` are matched as **exact tokens** (also `-x=<file>` for the value flags) by a normalizer that runs before cobra parses, and rewrites them to the long form. `-nop=<x>`, and `-no`, are usage errors (exit 64).
- `-h` stays help, `-c` stays the configuration list, `-o` stays `--output` (the rewrite matches only the exact tokens `-os` and `-os=<file>`).
- The single letters `-s`, `-p`, `-t` and `-n` are deliberately unused. A unit test covers the full set.
- A value flag with no value is a usage error (exit 64). Each setup flag may appear once.
- A setup flag, like `-cl`, makes the run non-interactive (the file is the consent). `--review` opts back into Review.
- `-nop` together with `-ps` (or `facman setup -ps …`) is a usage error (exit 64).

## Appendix C: TUI specification

**Flow:** `Scan → Plan → Review → Run → Summary`, plus **Manage** and **Files** tabs.

| Screen | Behaviour |
|---|---|
| **Scan** | Bootstrap check, then detectors run in parallel. Lists info-level findings (untrusted foreign taps, superseded tools such as Gemini CLI). |
| **Plan** | Table grouped by ownership state. Required missing packages pre-checked, optional ones not, foreign ones adopt-only. Shows `unavailable-here` rows greyed with reason and hint, and `conflict` rows with evidence. Choice groups are single-select (or none). Install options (for example Paperclip exposure) are asked here or on Review. With `-nop`, Paperclip does not appear. |
| **Review** | The consent gate. Shows every exact command, sudo needs, unverified scripts, dependency order, the shell-profile block, `configure:` steps with target files, trust steps, and validation results. Consent recorded with OS user and timestamp. |
| **Run** | Per-item spinners and live logs. Sudo steps hand the terminal to the child (`tea.ExecProcess`). |
| **Summary** | Per-item result, logs, export, and "installed, sign-in required" next steps. Lists suppressed items such as `paperclip: suppressed (not touched)`. |
| **Manage** | `managed` and `adopted` items only: upgrade, uninstall, reinstall, forget. Services show running/stopped/unhealthy with start, stop, restart, logs and **Change exposure**. Foreign items never appear. |
| **Files** | Lists configuration lists and setup files in the repo with validation status. |

| Key | Where | Action |
|---|---|---|
| `space` | Plan | Toggle selection |
| `a` | Plan | Adopt a foreign item |
| `i` | Plan | Info: source, why needed, which profile asked, and conflict evidence |
| `/` | Plan | Filter |
| `r` | Plan (conflict item) | Move aside, then install (consented), or remove through the owning tool |
| `s` | Plan, Review | Save the current answers as a configuration list (installs nothing) |
| `Enter` | Review | Consent |
| `e` | Files | Open in `$EDITOR`, revalidate on save |
| `t` | Files | New file from a template |
| `d` | Files | Diff against what is currently applied |

`# TODO:` Navigation keys between screens and tabs, and quit/back keys, are not specified in the design.

## Appendix D: Ownership states and transitions

| State | Meaning | `facman` may | Becomes |
|---|---|---|---|
| `absent` | Not detected | Propose install | `managed` (install) |
| `foreign` | Present, not installed by `facman` (own brew install, DMG, native, OS-bundled) | Report only. Offer Adopt | `adopted` (Adopt) |
| `managed` | Installed by `facman`, in inventory | Upgrade, uninstall, repair | `drifted`, `absent` |
| `adopted` | Was foreign, developer handed it over | Same as managed. Uninstall needs extra confirmation | `drifted`, `absent` |
| `drifted` | Recorded, but no longer detected or detected via a different backend. Also a developer-edited file `facman` wrote | Flag. Offer Reinstall or Forget (files: keep, overwrite or fail by policy) | `managed`, `absent` |
| `duplicate` | Two or more sources (for example claude-code by brew and natively) | Report which wins on PATH. Never auto-resolve | (developer resolves) |
| `conflict` | A file or symlink the backend does not own sits at its write path | Show evidence. Offer move-aside or removal via owner. Never overwrite or `--force` | `absent` then `managed` |
| `unavailable-here` | Wanted by the profile but not installable on this platform or without a display | Show greyed with hint. Skip with warning. Never blocks | (none) |
| `suppressed` | Excluded for this run by `-nop` (Paperclip) | Not touched. Listed in Summary | (unchanged) |

Conflict owner classes: this package, another brew item, npm (resolves into brew/npm `node_modules`, listed by `npm ls -g`), another installer (native Claude, Copilot script, `.pkg` receipt via `pkgutil --file-info`), orphan.

## Appendix E: Catalog v1

Defaults come from the `agentic-factory` profile (§5). The `base-dev` profile it includes holds the developer basics, and its exact contents are `# TODO:` (not shown in the design). "Opt" means `optional: true` (unchecked by default, `--with <id>` headlessly).

| Package ID | Kind | macOS source | Linux / WSL | Default | Key rules |
|---|---|---|---|---|---|
| `claude-code` | cli | cask `claude-code` (stable). Fallback npm `@anthropic-ai/claude-code` | same cask (x86_64, ARM64) | On | Detect brew/native/npm by path. Native updates itself, so report duplicates. Managed settings are checked, not set |
| `claude-desktop` | gui | cask `claude` | macOS-only: report with docs link. WSL: host hint | On | Self-updating |
| `copilot-cli` | cli | cask `copilot-cli` | same cask | On | Needs a Copilot seat and the org CLI policy |
| `github-copilot-app` | gui | cask `github-copilot-app` | Linux: same cask. WSL: host | Opt | Separate org policy. Not `github-copilot-for-xcode` or `github` (Desktop) |
| `antigravity-cli` | cli | cask `antigravity-cli` (`agy`) | same cask | On | Replaces Gemini CLI. A leftover `gemini` is reported `foreign` and superseded. Never installed |
| `codex` | cli | cask `codex` | same cask | On | npm `@openai/codex` alternative |
| `hermes-agent` | cli | formula `hermes-agent` (never bare `hermes`) | same formula | On | Apple Silicon bottle, Intel builds from source (needs rust). Upgrade via `brew upgrade`, not `hermes update`. Desktop app `Hermes.app` detected |
| `orca` | gui | tap cask `stablyai/orca/orca` (never bare `orca`) | Linux unverified. WSL: host | On | Item-level `brew trust --cask stablyai/orca/orca` step. Bundle ID to verify (`dev.orca.app`) |
| `t3-code` | gui | cask `t3-code` | Linux: same cask (x86_64 AppImage). WSL: host | Opt | Beta. Detect by bundle ID, because the app name includes "(Beta)" |
| `paperclip` | npm service | npm `paperclipai` pinned, `onboard --yes`, then `service install` | same | On | Requires Node ≥ 24.11 and corepack/pnpm. Not as root. Localhost by default. Backup before upgrade. Suppressible with `-nop` |
| `git` | cli | formula `git` | same. OS git counts as present | via base-dev | Never run `/usr/bin/git` before `xcode-select -p` |
| `gh` | cli | formula `gh` | same | via base-dev | |
| `tmux` | cli | formula `tmux` | same | via base-dev | |
| `visual-studio-code` | gui | cask `visual-studio-code` | Linux: report (Microsoft apt/rpm). WSL: host | via base-dev | `code` command linked by the cask |
| `iterm2` | gui | cask `iterm2` | `platforms: [darwin]` only | via base-dev | Never shown on Linux or WSL |
| `sqlite` | cli | formula `sqlite` (keg-only) | OS copy counts | via base-dev | `facman`'s own state does not depend on it |
| `node` | cli | formula `node@24`, `link: force` | same | via base-dev | `satisfies_if min_version 24.11` from any provenance (brew, nvm, fnm, asdf, Volta). Explicit dependency |
| `awscli` | cli | formula `awscli` | same | via base-dev | `reject_if version < 2.0` (pip v1 is `duplicate`/`foreign`). Credentials out of scope |
| `uv` | cli | formula `uv` | same | On | Astral script is the alternative |
| `jq` | cli | formula `jq` | same | On | macOS 15+ `/usr/bin/jq` counts as present (OS) |
| `ripgrep` | cli | formula `ripgrep` | same | On | Claude Code's bundled rg does not count |
| `container-runtime` | choice | `orbstack` (macOS only), `colima` (+ `docker`, `docker-compose`), or `docker-desktop` | Linux: Colima through brew, or report distro engine. WSL: Docker Desktop on host (host-report) | Opt, off | `satisfied_by_any_present`. Never installs a second runtime. Rancher Desktop conflicts with Docker Desktop |
| `session-manager-plugin` | cli | cask `session-manager-plugin` | macOS-only: report with AWS package link | Opt | Requires `awscli` |
| `gcloud-cli` | cli | cask `gcloud-cli` | same (x86_64, ARM64) | Opt | Pulls in `python@3.14` |
| `azure-cli` | cli | formula `azure-cli` | same | Opt | Large Python tree |

## Appendix F: File formats

### F1. Catalog entry (`catalog.yaml`, `apiVersion: facman/v1`)

| Field | Meaning |
|---|---|
| `display`, `category`, `kind` (`cli`\|`gui`) | Name, grouping, and platform behaviour |
| `detect.commands` / `detect.bundle_ids` / `detect.winget_id` | How to find it. `version: { cmd, regex }`. `provenance:` maps path globs to `native`, `brew`, `npm`, `nvm`, `fnm`, `volta`, `script` (templates like `{brew_prefix}`, `{npm_prefix}`) |
| `install.<darwin\|linux\|wsl>[]` | Ordered backends: `backend` (`brew-formula`, `brew-cask`, `npm`, `report`, `host-report`; later `winget-host`, `native-pkg`), `id`, `tap`, `trust: item`, `version` pin, `link: force`, `post_install`, `hint`. A missing block means not offered here. `wsl` inherits `linux` for CLIs |
| `version_policy` | `track` (upgrade via the backend), `pinned` (only where the backend supports it), `delegate` (the app self-updates) |
| `requires: [{ package, min_version }]` | Dependencies, ordered first by the planner |
| `satisfies_if`, `reject_if` | When another provenance or version counts as present, or must be rejected |
| `service:` | `manager` (auto: launchd or `systemd --user`), `install_cmd`, `health: { http, timeout }`, `options`, `backup_before_upgrade`, `run_as: invoking-user`, `required` |
| `choice[]` | Choice group alternatives with `platforms`, `host_on`, and `satisfied_by_any_present` |
| `platforms` | Restrict to platforms (for example `[darwin]`) |

### F2. Profile (`profiles/<factory>.yaml`)

`apiVersion`, `profile`, `description`, `include: [other-profiles]`, `packages:` (an ID, or `{ id: { optional: true } }`), `policy:` (`allow_unverified_scripts`, `allow_adopt`). A machine selects one or more profiles, and the union is the desired state.

### F3. Configuration list (`kind: ConfigurationList`)

| Block | Keys |
|---|---|
| `metadata` | `name`, `description`, `created_by`, `created_at` (filled in on save) |
| `catalog` | `source` (git URL), `ref` (tag), `sha256` |
| `platforms` | List. Apply refuses elsewhere |
| `packages[]` | `id`, optional `version` pin, `choice` (required for choice groups), `options` (risky values must be written), `setup: <ToolSetup path>` or inline `configure:`, `required` |
| `policy` | `foreign` (skip\|adopt\|fail, default skip), `unavailable_here` (warn\|fail), `allow_sudo` (package installs), `conflicts` (fail\|move_aside, default fail), `tap_trust` (for example `[cask:stablyai/orca/orca]`), `allow_whole_tap_trust` (default false), `config_drift` (keep\|overwrite\|fail, default keep), `format_mismatch` (warn\|fail) |
| `bootstrap` | `homebrew` (auto\|never), `node` (auto), `allow_sudo` (default true for bootstrap), `persist_shellenv` (default true), `prune` (default false) |

### F4. Tool setup spec (`kind: ToolSetup`)

`apiVersion: facman/v1`, `kind: ToolSetup`, `tool` (`hermes`\|`paperclip`\|`orca`\|`t3`), `configure:` steps:

| Step type | Fields | Behaviour |
|---|---|---|
| `files` | `from`, `to`, `template` (expand `${VAR}`) | Copy files. Hash recorded. A developer-edited file is not overwritten |
| `env_file` | `to`, `keys` (names only) | Write names with values taken from the environment |
| `command` | `run`, `once`, `requires_env` | Run a command. `once: true` is recorded and skipped later |
| `json_merge` | target file, JSON fragment | Deep-merge keys. Records merged keys so only they can be removed |

### F5. Per-tool seeding matrix (§6b)

| Tool | File-based? | Seeded | Applied by |
|---|---|---|---|
| Paperclip | Strong: company packages (`COMPANY.md`, `agents/<name>/AGENTS.md`, `projects/`, `skills/<name>/SKILL.md`, `tasks/`, `.paperclip.yaml`) | Agents, projects, skills, tasks, adapters, budgets, env var names | `paperclipai company import <path\|url> [--ref v] --target new --new-company-name … [--dry-run]`. Secret values never in the package. Imported agents start with heartbeats paused. Paperclip must be running |
| Hermes | Good: files under `~/.hermes` (`HERMES_HOME`) | `config.yaml` (with `${VAR}` expansion), `SOUL.md`, `skills/`, `cron/`, profiles, `.env` names | Copy templated files, and/or `hermes config set KEY VAL`. `hermes -p <profile>` |
| T3 Code | Settings only | `settings.json` under `T3CODE_STATE_DIR` (default `~/.t3/userdata`, filename `# TODO:` unconfirmed): `providerInstances`, `enableProviderUpdateChecks`. Env vars `T3CODE_PORT`, `T3CODE_HOST`, `T3CODE_AUTH_TOKEN`, `T3CODE_DISABLE_AUTO_UPDATE` | `json_merge`, then env vars |
| Orca | Partial | `orca.yaml` (`worktree.sharedDirectories`), `~/.orca/keybindings.json` | Script the CLI: `orca repo add --path …`, `orca repo set-base-ref …`, `orca automations create …`, `orca skills install …`, `orca serve service install` |

### F6. Validation and contract tests

| Stage | Check | Output |
|---|---|---|
| 1. Syntax | YAML or JSON parse | `file:line:col` |
| 2. Schema | JSON Schema for `facman` kinds and tool-native files | Offending key and line, for example `team.setup.yaml:14:5 configure[1].type: "file" is not one of files, env_file, command, json_merge` |
| 3. Semantics | IDs exist in the pinned catalog, picks valid, option values allowed, referenced assets exist, env var names declared, **no secret-looking values (error)** | Errors |
| 4. Version fit | Tool-native file's declared version range vs the pinned tool | Warning, error under `--strict` |
| 5. `--deep` | The tool's own checker: `paperclipai company import <pkg> --dry-run`, `hermes config check` against a temp `HERMES_HOME`, T3 parse plus key check | Errors |

Schemas live in `schemas/` (`facman/configuration-list.v1.json`, `facman/tool-setup.v1.json`, `hermes/config.<ver>.json`, `paperclip/company-package.<ver>.json`, `t3/settings.<ver>.json`, `orca/orca-yaml.<ver>.json`). Golden fixtures live in `testdata/contracts/<tool>/`. Orca and T3 schemas are `vendor-unofficial`. Contract tests: fixtures and template outputs validate (good pass, bad fail); in a Linux container (and a macOS runner for macOS-only tools) install the pinned tool through `facman`, apply the golden setup and run the tool's checker; nightly against latest releases (failure opens an issue, does not block).

## Appendix G: Bootstrap checks (§4a)

| Step | Check | TUI if missing | `-cl` if missing |
|---|---|---|---|
| 1. Platform | OS, arch, WSL (`WSL_DISTRO_NAME` or `microsoft` in `/proc/version`), display (`DISPLAY`/`WAYLAND_DISPLAY`), glibc | none | none |
| 2. Xcode CLT (macOS) | `xcode-select -p` | Folded into Homebrew install | Same, headless through `softwareupdate` |
| 2L. Linux prerequisites | `gcc`/`build-essential`, `procps`, `curl`, `file`, `git` | Prompt, then apt or dnf | Auto, needs non-interactive sudo |
| 3. Homebrew | `brew` on PATH, or at `/opt/homebrew`, `/usr/local`, `/home/linuxbrew/.linuxbrew` | Prompt with full command | Auto unless `bootstrap.homebrew: never` |
| 4. Shell environment | `brew shellenv` applied | Applied to process. Prompt before profile write | Applied. Profile write by `persist_shellenv` |
| 5. Node/npm | `node` ≥ 24.11, `npm` | Prompt only if a selected package uses npm | Auto if a listed package needs npm |

Linux prerequisites: Debian/Ubuntu `sudo apt-get update && sudo apt-get install -y build-essential procps curl file git`. Fedora/RHEL `sudo dnf group install -y development-tools && sudo dnf install -y procps-ng curl file git`.

Profile block (macOS `~/.zprofile`, Linux and WSL `~/.bash_profile` or `~/.profile`):

```sh
# >>> facman managed: homebrew >>>
eval "$(/opt/homebrew/bin/brew shellenv)"
export PATH="$(brew --prefix node@24)/bin:$PATH"
# <<< facman managed: homebrew <<<
```

GitHub Actions: `ubuntu-24.04` has Homebrew at `/home/linuxbrew/.linuxbrew` (not on PATH), default Node 22 (Node 24 in the tool cache), and a non-root user with passwordless sudo. `macos-*` has Homebrew on PATH. Container jobs and self-hosted runners usually have none and fall back to the full bootstrap.

## Appendix H: Exit codes

| Code | Meaning | Examples |
|---|---|---|
| 0 | Success, or already compliant | |
| 1 | An operation failed | install command failed |
| 2 | Drift found | `apply --check` and the machine differs |
| 3 | Validation error | bad file, missing env var name value under `-cl`, setup flag for an uninstalled tool, format mismatch under `-cl` |
| 4 | Bootstrap failed | no non-interactive sudo, UID 0, unknown distro, Homebrew missing with `bootstrap.homebrew: never` |
| 64 | Usage error | value flag with no value, `-nop=<x>`, `-nop` with `-ps`, bare `-cl` |

## Appendix I: Inventory schema (§6, §6a)

```sql
packages(id TEXT PK, state TEXT, backend TEXT, backend_ref TEXT,
         installed_version TEXT, install_path TEXT,
         installed_at, installed_by, adopted_at, manifest_rev TEXT,
         scope, bootstrap BOOL)               -- scope: per-machine vs per-user
operations(id INTEGER PK, started_at, finished_at, package_id, op TEXT,   -- install|upgrade|uninstall|adopt|forget
           commands_json TEXT, exit_code INT, status TEXT, log_path TEXT, consent_by TEXT)
detections(scanned_at, package_id, found BOOL, version, provenance, path)
services(package_id, manager TEXT,            -- launchd | systemd-user
         unit_path, port, exposure TEXT,      -- local | remote
         last_health, last_backup_path)
meta(key PK, value)                           -- schema_version, machine_id, last_manifest_rev
```

Also recorded: file hashes for every file `facman` writes, tap trust grants (with the package that needed them), moved-aside files (with their backup path), merged JSON keys, the managed shell-profile block, and `suppressed=` notes in `consent_by`. `consent_by` values: OS user and timestamp (TUI), `config:<path>@<sha>` (`-cl`), `setup:<path>@<sha>` (setup flags). `# TODO:` the design lists these records but not their exact columns.

## Appendix J: Paths, environment and platform matrix

| Item | macOS | Linux and WSL |
|---|---|---|
| State DB | `~/Library/Application Support/facman/state.db` | `# TODO:` XDG state path (design names only "the XDG equivalent") |
| Backups (moved-aside files, Paperclip upgrade backups) | `~/Library/Application Support/facman/backups/<timestamp>/` | XDG equivalent |
| Homebrew prefix | `/opt/homebrew` (Apple Silicon), `/usr/local` (Intel) | `/home/linuxbrew/.linuxbrew` |
| Paperclip data | `~/.paperclip/instances/default/` (`secrets/master.key`, `db`). Port 3100 | same |
| Hermes | `~/.hermes` or `HERMES_HOME` | same |
| T3 Code state | `T3CODE_STATE_DIR` (default `~/.t3/userdata`) | same |

Environment variables read or honored: `HTTPS_PROXY` and custom CA vars, `HERMES_HOME`, `T3CODE_STATE_DIR`, `T3CODE_PORT`, `T3CODE_HOST`, `T3CODE_AUTH_TOKEN`, `T3CODE_DISABLE_AUTO_UPDATE`, secret names declared by specs (for example `ANTHROPIC_API_KEY`, `OPENROUTER_API_KEY`), `EDITOR`. Set for Paperclip remote exposure: `PAPERCLIP_DEPLOYMENT_MODE=authenticated`, `PAPERCLIP_DEPLOYMENT_EXPOSURE=public`. Never set: `HOMEBREW_NO_REQUIRE_TAP_TRUST`.

Availability by platform (summary of design §2a): CLI formulae and casks work on macOS, Linux and WSL. GUI apps are macOS casks. On Linux several GUI casks are macOS-only (Claude desktop, VS Code, iTerm2, the Session Manager plugin). On WSL, GUI apps live on the Windows host and are reported in v1. Platform-specific gotchas: Linux bootstrap needs the distro package manager once, Homebrew bottles need glibc ≥ 2.39, keep `facman` state and Homebrew on the Linux filesystem (never `/mnt/c`), Linux/WSL distro `git` and `sqlite3` count as present (OS), and Paperclip as a user service needs `loginctl enable-linger <user>` in CI.

## Appendix K: Design cross-reference

| Topic | Design section |
|---|---|
| Purpose, non-goals | §1 |
| Package research and catalog | §2 |
| Platform matrix and gaps | §2a |
| Ownership states, conflicts, tap trust | §3, §3a, §3b |
| Architecture, backend interface, detection, sudo | §4 |
| Startup bootstrap | §4a |
| Manifest formats (catalog, profiles, choice groups) | §5 |
| State store | §6 |
| Managed services (Paperclip) | §6a |
| Seeding tool configuration | §6b |
| TUI flow | §7 |
| Configuration lists, setup flags, `-nop`, CI specifics | §7a |
| Templates, validation, contract tests | §7b |
| Language and framework choice | §8 |
| Fleet and lab considerations | §9 |
| Windows notes | §10 |
| Phased plan | §11 |
| Open questions | §12 |
| Install and usage | §13 |
