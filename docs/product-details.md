# Product Details

This page describes the settled behaviour of `facman`, the **Agentic Software Factory Manager**. It covers what Increment 1 (Phase 0 and Phase 1, the MVP) ships today, and where the rest of the product is designed. Requirements and acceptance criteria are in the [Increment 1 spec](../specs/archive/261114-software-factory-manager/spec.md). The full product is in the [PRD](../specs/archive/261114-software-factory-manager/software-factory-manager-PRD.md) and the [design](design/software-factory-installer-design.md).

## Increment 1: what ships

### Principles, as built

1. **Detect first.** Every command starts with a read-only bootstrap check and scan. A detector that fails or times out marks its packages "detection incomplete", and those are never planned.
2. **Propose only what is missing.** Only `absent` packages that are wanted by the selected profiles and installable here are planned. `foreign`, `duplicate` and OS-bundled installs are reported and left alone.
3. **Ask before acting.** The only consent is `Enter` on the TUI's Review screen or `y` at `apply`'s `Proceed? [y/N]` (default No). The consent covers exactly the commands shown. There is no `--yes`.
4. **Manage only what it installed.** Only installs that `facman` ran become `managed` records. Each one carries the consent string (`user@time`).

### Platforms

| Platform | Behaviour |
|---|---|
| macOS (arm64, amd64) | Primary target. Formulae and casks through Homebrew. The CLT state is checked, and the `/usr/bin/git` shim is never run without the CLT. |
| Linux (amd64, arm64) | Homebrew on Linux, found at `/home/linuxbrew/.linuxbrew` even off `PATH`. Casks are offered only with a display. glibc older than 2.39 warns. Missing distro prerequisites are reported with the exact `apt` or `dnf` command, and `apply` exits 4. |
| WSL 2 | As Linux. Windows GUI apps are `unavailable-here` with a `winget` hint to run on the host. |
| Root (UID 0) | Refused, exit 4, except for `--version`, `--about` and `--help`. |

### Commands

| Command | Behaviour |
|---|---|
| `facman` | TUI: Scan → Plan → Review → Run → Summary, install only. Default profile `agentic-factory`. Needs a terminal on stdout (otherwise it prints the about text and exits 0) and on stdin (otherwise exit 64). |
| `scan` | Every catalog package with state, provenance, version, path, the PATH winner for duplicates, and the reason for `unavailable-here`. |
| `plan --profile` | Exact ordered commands, dependencies first, with sudo, unverified-script, tap and trust marks. Lists exclusions with their reasons. |
| `apply --profile` | Same plan, then consent, then the run. Each action is recorded before it runs. A failure skips its dependents and the run continues (exit 1). The first signal stops after the current command. `--dry-run` changes nothing. |
| `status` | Managed and adopted records with provenance, plus foreign, drifted and duplicate packages. |
| `doctor` | Conflict and tap-trust check for the whole catalog, without planning. Exit 2 when something needs fixing. |

Every command takes `--json` (report schema v1) and `--catalog <dir>`. Full flag and exit-code reference: [user-docs/configuration.md](../user-docs/configuration.md).

### Catalog and profiles

- Catalog v1 is built in: 30 entries. That is the 25 PRD Appendix E packages, plus the container-runtime choice members (`orbstack`, `colima`, `docker-desktop`), the `docker` and `docker-compose` helpers that Colima requires, and the detection-only `gemini-cli`. When Gemini CLI is found, the scan reports that it is superseded by Antigravity CLI.
- Profiles: `agentic-factory` (includes `base-dev`; optional items off by default) and `base-dev`.
- Choice groups: `container-runtime` is single-select. Headless runs pick a member with `--with <member>`.
- Five entitlement tools are marked `sign_in_required`, and the Summary lists their sign-in steps.

### Ownership and safety

- **States:** `absent`, `managed`, `adopted` (shown; adopting arrives in Increment 2), `foreign`, `drifted`, `duplicate`, `conflict`, `unavailable-here`.
- **Conflicts:** before planning a brew item, `facman` checks the paths it would link. Unowned files there make it `conflict`, with evidence (the `ls -l` line, file type, symlink target, owner class) and a fix hint. It is left out with its dependents, and the run exits 1. `facman` never uses `--force` and never runs `brew link`.
- **Tap trust (Homebrew 6 and later; CI runs Homebrew 7.0.11):** a third-party tap item gets three separate, reviewed actions: `brew tap`, `brew trust --cask <tap>/<item>` (item level, never the whole tap) and the install. `HOMEBREW_NO_REQUIRE_TAP_TRUST*` is removed from every child.
- **Node:** `node@24` is installed keg-only and never force-linked. Children get its `bin` first on `PATH`, and the Summary prints the line for your shell profile.
- **Homebrew bootstrap:** if Homebrew is missing, the plan starts with the installer. The installer is downloaded to a private temp file, marked "unverified script" (Homebrew publishes no checksum) and run with the terminal. After it runs, `facman` re-detects and only drops actions that are no longer needed.
- **Concurrency:** one mutating run at a time (`flock`). Read-only commands never block.
- **Recovery:** interrupted operations are reconciled at the next start, and the next `apply` finishes the job.

### Audit and state

The state DB (SQLite) holds `packages` (managed and adopted records) and `operations`, an append-only log of every action with its commands, consent, status, exit code and log path. There is one log file per operation, pruned after 90 days. Paths are in [user-docs/configuration.md](../user-docs/configuration.md#state-and-logs).

### Known limits in Increment 1

- No upgrade, uninstall, adopt, forget, restore or move-aside. Conflicts are refused, not resolved.
- No npm backend, so Paperclip is `unavailable-here`.
- No configuration lists (`-cl`), so unattended runs are not possible yet. `apply` always needs a terminal.
- No templates, `validate`, setup flags (`-hs`, `-ps`, `-os`, `-ts`), managed services or `export`.
- The bundle IDs for the GitHub Copilot app, T3 Code and Orca, and the `brew info` and `brew trust` output shapes, still need confirming on a real lab Mac (spec I1-16).

## Later increments

| Area | Increment | Design section |
|---|---|---|
| Upgrade, uninstall, adopt, forget, move-aside and `restore`; npm backend; Paperclip as a managed service; `-nop` | 2 | [§3](design/software-factory-installer-design.md#3-core-concept-ownership-states), [§6a](design/software-factory-installer-design.md#6a-managed-services-paperclip) |
| Configuration lists and unattended runs (`-cl`, `--check`, `--set`), `config` | 3 | [§7a](design/software-factory-installer-design.md#7a-saved-configurations-and-unattended-runs---configuration-list) |
| Templates, schemas and `facman validate` | 4 | [§7b](design/software-factory-installer-design.md#7b-file-formats-template-validate-and-keep-them-correct-j-2026-11-14) |
| Per-tool setup flags (`-hs`, `-ps`, `-os`, `-ts`), `setup`, `export` | 5 | [§6b](design/software-factory-installer-design.md#6b-seeding-tool-configuration-from-files-agents-skills-settings) |
| Full TUI flow (Manage and Files tabs) | 2 to 5 | [§7](design/software-factory-installer-design.md#7-tui-flow) |
| Phased plan | — | [§11](design/software-factory-installer-design.md#11-phased-plan) |

Back to [docs](README.md).
