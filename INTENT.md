# Intent

## What we are building

**`sfi`** (software-factory installer; working name) is a single Go binary with a terminal UI and a headless CLI. It gives every lab machine the same toolset for taking part in one or more software factories, and it keeps that toolset current without trampling what developers already installed.

The repo is `software-factory-manager`. The product inside it is `sfi`.

## Who it is for

- **Developers on lab machines.** These are mostly single-user Macs where the developer owns Homebrew and runs `sfi` as themselves. They want the factory toolset without losing their own setup.
- **Build machines and CI runners.** These are mostly headless Linux. They need the same toolset with no prompts, driven by a file checked into git.
- **Factory and lab owners.** They declare what a factory needs once (catalog, profiles, setup files) and get consistent, auditable machines.

## Why

Lab machines arrive in every state. Some tools are installed through Homebrew, some by vendor scripts, some are bundled with the OS, and some are installed twice. Hand-run install docs drift, and blunt scripts overwrite things developers care about. We want one tool that **detects first, proposes only what is missing, asks before acting, and only manages what it installed**.

## What it does

1. **Detects** each catalog package and how it got there: brew, cask, app bundle, vendor script, npm, OS, or version manager.
2. **Plans** only the missing pieces and shows the exact commands.
3. **Asks for consent**, then installs through pluggable backends.
4. **Records** what it installed in a local SQLite inventory, and later upgrades or removes **only what it owns**.
5. **Seeds tool configuration** from checked-in files: Hermes, Paperclip, Orca and T3 Code setup.
6. **Runs unattended** from a configuration list (`sfi apply -cl <file>`). The reviewed file is the consent.

## The toolset (catalog v1)

| Group | Packages |
|---|---|
| Agents and agent UIs | Claude Code (`claude-code`), Claude desktop, GitHub Copilot CLI, GitHub Copilot app, Codex CLI, Antigravity CLI (`agy`, replaces Gemini CLI), Hermes Agent (`hermes-agent`), Orca (`stablyai/orca/orca`), T3 Code, Paperclip (npm, run as a managed service) |
| Developer basics | git, GitHub CLI (`gh`), tmux, VS Code, iTerm2, sqlite, Node.js (`node@24`), uv, jq, ripgrep |
| Cloud | AWS CLI v2; optional: AWS Session Manager plugin, Google Cloud CLI, Azure CLI |
| Optional runtime | One container runtime from a choice group: Docker Desktop, OrbStack, or Colima. Default off. |

Codex, uv, jq and ripgrep are on by default. Optional items are off by default.

## Key decisions

- **Homebrew everywhere.** Homebrew is the primary backend on macOS, Linux and Windows through WSL 2. npm is the second backend, for Paperclip. macOS is the primary target.
- **Consent first.** `sfi` installs only what is missing, and only after the developer agrees. In unattended runs, the checked-in configuration list is the consent and is recorded with its path and commit.
- **Own only what we install.** Each package has an explicit ownership state: absent, foreign, managed, adopted, drifted, duplicate, or conflict. Foreign tools are reported, never changed, unless the developer adopts them. Conflicting files are never overwritten or forced.
- **Safest choice is the default.** A risky option needs an explicit choice in the TUI or in the file. Paperclip listens on localhost unless remote exposure is written into the configuration. Third-party taps need explicit trust.
- **Linux CI first for `-cl`.** Headless runs will mostly be on Linux CI, so the Linux bootstrap (Linuxbrew, non-root user, distro prerequisites) ships with configuration lists.
- **Bootstrap on every launch.** `sfi` checks for Homebrew and Node/npm on every launch. The TUI prompts for them, and `-cl` installs them automatically.
- **Files are contracts.** Every file format has a template, a schema and `sfi validate`. Each build runs contract tests to check that the formats still match the tools' real behaviour.
- **Go + Bubble Tea**, as a single static binary with no runtime. The tool sets up machines that may have nothing on them.

The full reasoning is in [docs/design/](docs/design/software-factory-installer-design.md), and decisions are logged in [docs/architecture-decision-record.md](docs/architecture-decision-record.md).

## Non-goals (v1)

- Not an MDM. It does not replace Jamf or Intune, though they can distribute it.
- No central server. Fleet reporting is a JSON export.
- No native Windows backend. Windows is served through WSL 2. Windows-host GUI apps are reported, with WinGet hints.
- No managing tools that `sfi` did not install, and no auto-resolving duplicates.
- No secrets in configuration files. Files list env var names only.
- No multi-user brew ownership. `sfi` assumes single-user machines and stops if the brew prefix belongs to someone else.

## How we work on it

The design comes before code: read [docs/design/](docs/design/) and [AGENTS.md](AGENTS.md) first. Features follow the PRD → spec → implementation flow in `specs/`. Every change updates its docs in the same PR. This file changes only when the product's purpose, audience, scope or key decisions change.
