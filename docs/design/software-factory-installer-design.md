# Software Factory Installer: Design (v0.1 draft)

*Status: draft v0.2 for review by J · 2026-11-14 · Working name: `sfi` (rename freely)*

*v0.2 changes: Homebrew on macOS, Linux and WSL (§2a). Corrected: hermes has a Homebrew formula (`hermes-agent`). Confirmed: paperclip has none. Gemini CLI replaced by Antigravity CLI. Single-user machines. Paperclip runs as a managed service, with network exposure chosen at install time (§6a). Added AWS CLI. Node.js is now an explicit catalog entry. Added Codex CLI, uv, jq and ripgrep (default on), and a container runtime, the AWS Session Manager plugin, gcloud and az (optional, default off). Added saved configuration lists and `--configuration-list`/`-cl` for prompt-free build machines (§7a), and install and usage docs including running from the repo (§13). Added the startup bootstrap for Homebrew and npm: prompted in the TUI, automatic with `-cl` (§4a). `-cl` is designed Linux-CI-first (§7a). Added seeding of agents and configuration from files (§6b), and per-tool setup flags `--hermes-setup`/`-hs`, `--paperclip-setup`/`-ps` and `--orca-setup`/`-os` and `--t3-setup`/`-ts` (§7a). Added `sfi template` and `sfi validate`, the format-contract rule for every build (§7b), and user docs for each file (`user_docs/`).*

## 1. Purpose

Lab machines need a consistent toolset to take part in several software factories. Developers already own many of these machines and have installed some tools themselves. `sfi` is a terminal UI (TUI) that:

1. **Detects** which tools in a declared manifest are already present, and how they got there (Homebrew, app bundle, vendor installer, npm, OS-bundled).
2. **Proposes** only the missing ones.
3. **Asks the developer for consent** before changing anything, and shows the exact commands it will run.
4. **Installs** through pluggable backend installers. **Homebrew is the primary backend on all three platforms**: macOS, Linux, and Windows through WSL 2. Section 2a covers what that means for GUI apps, which Homebrew cannot always install on Linux or WSL.
5. **Keeps a local inventory** of what it installed. It upgrades and uninstalls **only what it owns**, and never touches tools the developer installed on their own unless the developer explicitly hands them over.

### Non-goals (v1)

- Not an MDM. It does not replace Jamf or Intune, and it can be distributed by them.
- No central server in v1. Fleet reporting is a file export (§9).
- No configuration of the tools beyond install (auth, settings, MCP servers). Section 11 lists this as a likely v2 "post-install hooks" feature.
- No native Windows backend (Chocolatey or WinGet) in v1. Windows machines are served through WSL 2 + Homebrew. Windows-side GUI apps are *reported* with install hints, and a WinGet "host" backend is a later add-on (§2a, §10).
- macOS remains the first and primary target. Linux and WSL follow in the phase plan (§11).

---

## 2. Package research: what each item really is and how to install it

Researched 2026-11-14. "Cask" means a Homebrew cask (GUI app or prebuilt binary). "Formula" means a Homebrew formula.

| # | Requested | What it actually is (assumption flagged ⚠) | Recommended macOS source | Alternatives | Detection signals | Notes and risks |
|---|---|---|---|---|---|---|
| 1 | **hermes** | ⚠ **Hermes Agent** by Nous Research, an open-source agent CLI (`hermes`), plus an optional desktop app (`Hermes.app`) | **Formula** `hermes-agent` (homebrew-core; binaries `hermes`, `hermes-acp`, `hermes-agent`) | Vendor script `install.sh`; desktop DMG/ZIP (Apple Silicon only) | `hermes` on PATH, then resolve it: a `{brew_prefix}/Cellar/hermes-agent` path means brew, `~/.hermes/hermes-agent` plus `~/.local/bin/hermes` means script. Also `/Applications/Hermes.app`. | *Corrected 2026-11-14:* there **is** a core formula. The formula name is `hermes-agent`; bare `hermes` is not a formula. Bottles exist for Apple Silicon macOS and Linux x86_64/ARM64, but **not Intel Macs**, which would build from source (needs rust). It pulls a heavy dependency tree (python@3.14, node, ripgrep and others). On a brew install, upgrade with `brew upgrade` rather than `hermes update`, so the two update paths don't fight. Detection must tell a brew install from a script install and flag `duplicate` if both exist. |
| 2 | **paperclip** | ⚠ **Paperclip** (paperclipai/paperclip), a self-hosted Node.js server and React UI for orchestrating teams of agents | **npm**: `npx paperclipai@<pin> onboard --yes`, or `npx paperclipai install` for the managed, updatable install | Docker image and Compose file in the repo | global `paperclipai`, `~/.paperclip/instances/default/`, port 3100 | **No Homebrew path** (re-checked 2026-11-14: no homebrew-core formula, no cask, no official tap; the official docs list only npx and Docker). The only desktop app is unofficial and community-maintained. It needs **Node ≥ 24.11** and pnpm (through corepack), so `node` becomes a dependency. It must **not run as root**, because embedded Postgres refuses to. Back up `~/.paperclip/.../secrets/master.key`, which encrypts every stored API key. It runs a server with embedded Postgres. **Decision (J, 2026-11-14): `sfi` runs it as a managed service** (§6a). |
| 3 | **orca** | ⚠ **Orca** by Stably AI, a desktop "agent development environment" that runs Claude Code, Codex and others in parallel worktrees, with a CLI | **Tap cask**: `brew install --cask stablyai/orca/orca` | DMG from orca.dev | `/Applications/Orca.app` (verify the bundle ID), `orca` CLI | **Trap:** bare `orca` in Homebrew is an **unrelated Plotly cask**. The manifest must use the full tap path. Using a third-party tap means `sfi` must manage taps too. |
| 4a | **Claude desktop** | Anthropic's Claude desktop app | **Cask** `claude` | DMG | `/Applications/Claude.app`, bundle `com.anthropic.claudefordesktop` | Updates itself (`auto_updates true`). Homebrew upgrades are a no-op unless forced. |
| 4b | **Claude CLI** | Claude Code (`claude`) | **Cask** `claude-code` (stable channel, about a week behind latest) | `claude-code@latest` cask, native script, npm `@anthropic-ai/claude-code` | `claude` on PATH. Resolve the symlink to tell installs apart: `~/.local/share/claude/versions` means native, a `$(brew --prefix)/Caskroom` path means brew, an npm prefix means npm. | Recommend the **brew cask** so `sfi` controls the version. A native install updates itself and fights central version control. For the enterprise, pair it with Claude Code *managed settings* (`requiredMinimumVersion` and channel). It is common to find **two installs on one machine**, so detection must report duplicates. |
| 5a | **GitHub Copilot CLI** | `copilot` CLI | **Cask** `copilot-cli` | npm `@github/copilot` (Node ≥ 22), `gh.io/copilot-install` script | `copilot` on PATH, resolved the same way | It needs a Copilot seat, and for org users the **Copilot CLI policy** must be enabled. It updates itself. |
| 5b | **Copilot desktop** (J was unsure) | **It exists.** The **GitHub Copilot app** went GA in June 2026 and is available on all plans since July 2026. It is built on Copilot CLI. | **Cask** `github-copilot-app` | Download from github.com/features/ai/github-app | `/Applications/GitHub Copilot.app` | Business and Enterprise orgs control it with a separate **"GitHub Copilot app" policy** (on by default). Do not confuse it with `github-copilot-for-xcode` or the `github` (GitHub Desktop) cask. |
| 6 | **Gemini CLI → Antigravity CLI** | **Decision (J, 2026-11-14): replace Gemini CLI with Google Antigravity CLI** (`agy`) | **Cask** `antigravity-cli` (macOS 12+, Linux x86_64/ARM64) | — | `agy` on PATH; also detect a leftover `gemini` (npm `@google/gemini-cli` or the deprecated formula) | Google moved consumer users to Antigravity CLI on 2026-06-18, and the `gemini-cli` formula is disabled on 2026-12-18. `sfi` never installs Gemini CLI. If it finds one it reports it as `foreign` with a note that it has been superseded, and leaves it alone unless the developer adopts it and asks to remove it. |
| 7 | **T3 desktop** | ⚠ **T3 Code** (pingdotgg/t3code, t3.codes), a minimal GUI for AI code agents | **Cask** `t3-code` | `t3-code@nightly` | `/Applications/T3 Code (Beta).app` | Beta (v0.1.x). The app name includes "(Beta)", so detect by bundle ID, not by name. If J meant *T3 Chat*, there is no desktop app to install (Q2). |
| 8 | **VS Code** | Visual Studio Code | **Cask** `visual-studio-code` | — | `/Applications/Visual Studio Code.app`, `code` on PATH | Updates itself. The `code` shell command is linked by the cask. |
| 9 | **tmux** | tmux | **Formula** `tmux` | — | `tmux` on PATH | Straightforward. |
| 10 | **iTerm2** | iTerm2 | **Cask** `iterm2` | — | `/Applications/iTerm.app` (bundle `com.googlecode.iterm2`) | Updates itself. |
| 11 | **GH CLI** | GitHub CLI | **Formula** `gh` | — | `gh` on PATH | Straightforward. |
| 12 | **git CLI** | git | **Formula** `git` | Apple git through the Command Line Tools (CLT) | `git` on PATH, but see the gotcha | **Gotcha:** `/usr/bin/git` is a *shim*. Running it on a Mac without CLT pops a GUI install dialog. Detect with `xcode-select -p` first and never run `/usr/bin/git` blindly. Apple git counts as "present (OS)". Decide whether the factory requires a minimum version (Q4). |
| 13 | **sqlite** | SQLite CLI | **Formula** `sqlite` | Ships with macOS (`/usr/bin/sqlite3`) | `sqlite3` on PATH | The brew formula is **keg-only**: it is not linked onto PATH, which shadows the system copy only if you add it to PATH yourself. Default: treat the OS copy as satisfying the requirement unless a minimum version is set. Separate from this, `sfi`'s own state store embeds SQLite (§6), so `sfi` does not depend on this package. |
| 14 | **AWS CLI** *(added by J, 2026-11-14)* | AWS CLI v2 (`aws`) | **Formula** `awscli` | AWS's official `.pkg` (macOS) or zip installer (Linux) | `aws` on PATH; `aws --version` must report `aws-cli/2.x` | Works the same on macOS, Linux and WSL. Watch for an old **v1** `aws` installed through pip, which shadows v2 on PATH; detect it and report it as `duplicate`. Credentials (`~/.aws`, SSO profiles) are out of scope for v1. |
| 15 | **Node.js** *(explicit prerequisite)* | Node.js runtime, plus pnpm through corepack | **Formula** `node@24` (pinned LTS line, which satisfies Paperclip's ≥ 24.11) | `node` (latest) | `node --version`; provenance tells brew from nvm, fnm, asdf or Volta | Now a **first-class catalog entry**, not an implicit dependency. Packages declare `requires: node`, and the planner orders it first. `node@24` is keg-only, so `sfi` must put it on PATH (`brew link --force node@24` or a shell snippet). If a version manager already supplies Node ≥ 24.11, it counts as satisfied and `foreign` (Q5). Paperclip also needs **pnpm**, which `sfi` enables through `corepack enable` as part of Paperclip's install, not as a separate package. |
| 16 | **Codex CLI** *(added 2026-11-14)* | OpenAI's coding agent (`codex`) | **Cask** `codex` | npm `@openai/codex` | `codex` on PATH | macOS and Linux (x86_64, ARM64). Orca and T3 Code both drive it alongside Claude Code. **Default on.** |
| 17 | **uv** *(added)* | Astral's Python package and project manager (`uv`, `uvx`) | **Formula** `uv` | Astral install script | `uv` on PATH; provenance tells brew from `~/.local/bin` (script) | Needed to launch most Python MCP servers through `uvx`. All platforms. **Default on.** |
| 18 | **jq** *(added)* | JSON processor | **Formula** `jq` | — | `jq` on PATH | macOS 15+ ships `/usr/bin/jq`, which counts as present (OS). All platforms. **Default on.** |
| 19 | **ripgrep** *(added)* | Fast search (`rg`) | **Formula** `ripgrep` | — | `rg` on PATH | Claude Code bundles its own rg, so that one doesn't count as present. All platforms. **Default on.** |
| 20 | **Container runtime** *(added, choose one)* | One of Docker Desktop, OrbStack, or Colima + Docker CLI | **Choice group** (§5): `docker-desktop` cask, `orbstack` cask, or formulae `colima` + `docker` + `docker-compose` | Rancher Desktop (`rancher` cask, conflicts with Docker Desktop) | `docker` on PATH, `docker context ls`, the app bundles, a running daemon socket | **Default off** (J: Docker Desktop is heavy). Docker Desktop needs a paid licence at larger companies. OrbStack is macOS-only. Colima works on macOS and Linux. On WSL, Docker Desktop lives on the **Windows host** with WSL integration (host-report). Native Linux can use Colima or the distro's Docker Engine. Detect an existing runtime first and never install a second one. |
| 21 | **AWS Session Manager plugin** *(added, optional)* | `session-manager-plugin` for `aws ssm start-session` | **Cask** `session-manager-plugin` | AWS's `.deb`/`.rpm` on Linux | `session-manager-plugin` on PATH | **macOS-only cask.** Linux and WSL are report-only, with a link to AWS's package. Useful for remote access to EC2 without SSH. **Default off.** Requires `awscli`. |
| 22 | **Google Cloud CLI** *(added, optional)* | `gcloud` | **Cask** `gcloud-cli` (formerly `google-cloud-sdk`) | Google's install script | `gcloud` on PATH | macOS and Linux (x86_64, ARM64). Pulls in `python@3.14`. **Default off.** |
| 23 | **Azure CLI** *(added, optional)* | `az` | **Formula** `azure-cli` | Microsoft's script or apt repo | `az` on PATH | All platforms. Large Python dependency tree. **Default off.** |

### Implicit dependencies the manifest must model

- **Xcode Command Line Tools.** Homebrew needs them. They are a bootstrap prerequisite and may need a GUI click or `softwareupdate`.
- **Homebrew itself.** Checked and installed by the startup bootstrap (§4a). Note the prefix: `/opt/homebrew` on Apple Silicon, `/usr/local` on Intel, `/home/linuxbrew/.linuxbrew` on Linux and WSL.
- **Node.js** is now an explicit catalog entry (row 15) rather than an implicit dependency. It is needed by Paperclip and by any npm fallback path. Paperclip's ≥ 24.11 requirement sets the floor. If the developer already has Node through nvm, fnm, asdf or Volta, `sfi` must detect it and respect it rather than install a second Node (Q5).
- **Tap** `stablyai/orca` for Orca.

### Packages with no Homebrew path

Only `paperclip` (npx/npm) has no Homebrew path. Gemini CLI is replaced by the `antigravity-cli` cask. npm is approved by J, so v1 needs an **npm backend** alongside Homebrew, and today paperclip is its only user. *Correction:* hermes does have a formula (`hermes-agent`), so the **script backend drops out of v1** and stays as a later option for vendor-only tools.

---

## 2a. One backend, three platforms: Homebrew on macOS, Linux and WSL

J's direction (2026-11-14) is to use Homebrew everywhere. Homebrew on Linux is Tier 1 when the distro is in standard support with glibc ≥ 2.39 (for example Ubuntu 24.04+), on x86_64 and ARM64. WSL 2 is simply Linux to Homebrew. Homebrew 6 even labels WSL in `brew config`, and Microsoft's Windows developer guidance recommends Homebrew for WSL. **Formulae (CLIs) work the same on all three platforms. GUI apps are where the platforms diverge.**

### Availability matrix (checked on formulae.brew.sh, 2026-11-14)

| Package | Kind | macOS | Linux (desktop) | WSL 2 | What `sfi` does where Homebrew can't install it |
|---|---|---|---|---|---|
| tmux, gh, git, sqlite, node@24, awscli, uv, jq, ripgrep, azure-cli | CLI formula | ✅ formula | ✅ formula | ✅ formula | — |
| Claude Code | CLI cask | ✅ `claude-code` | ✅ `claude-code` (x86_64, ARM64) | ✅ same as Linux | — |
| Copilot CLI | CLI cask | ✅ `copilot-cli` | ✅ `copilot-cli` (x86_64, ARM64) | ✅ same as Linux | — |
| Codex CLI, Google Cloud CLI | CLI cask | ✅ `codex`, `gcloud-cli` | ✅ (x86_64, ARM64) | ✅ same as Linux | — |
| AWS Session Manager plugin | CLI cask | ✅ `session-manager-plugin` | ❌ macOS-only cask | ❌ | Report it, with a link to AWS's .deb/.rpm |
| Container runtime (choice) | GUI cask or formulae | ✅ Docker Desktop, OrbStack or Colima | ⚠ Colima or distro Docker Engine | ⚠ Docker Desktop on the **Windows host** | WSL: host-report. Linux: Colima through brew, or report the distro engine. |
| Antigravity CLI (`agy`), replaces Gemini CLI | CLI cask | ✅ `antigravity-cli` | ✅ (x86_64, ARM64) | ✅ | — |
| Hermes Agent | CLI formula | ✅ `hermes-agent` (Apple Silicon bottle; Intel builds from source) | ✅ `hermes-agent` (x86_64, ARM64) | ✅ same as Linux | — |
| Paperclip | npm | ✅ npm | ✅ npm | ✅ npm | Server only, no GUI. Reached from a browser on the Windows host through WSL localhost forwarding. |
| GitHub Copilot app | GUI cask | ✅ `github-copilot-app` | ✅ same cask (x86_64, ARM64) | ⚠ install on **Windows host** | WSL: report it with the hint `winget …`, or use the later host backend |
| T3 Code | GUI cask | ✅ `t3-code` | ✅ same cask (x86_64 AppImage; ARM64 not listed) | ⚠ install on **Windows host** | Same as above |
| Orca | GUI tap cask | ✅ `stablyai/orca/orca` | ⚠ unknown; check the tap's Linux support | ⚠ install on **Windows host** | Same as above |
| Claude desktop | GUI cask | ✅ `claude` | ❌ macOS-only cask | ❌ install on **Windows host** | Linux: report it, with a link to Anthropic's Linux desktop install docs. WSL: host hint. |
| VS Code | GUI cask | ✅ `visual-studio-code` | ❌ macOS-only cask | ❌ install on **Windows host** | Linux: report it, with Microsoft's apt/rpm repo as the hint. WSL: VS Code runs on Windows, and its `code` command plus the WSL extension reach into the distro. |
| iTerm2 | GUI cask | ✅ `iterm2` | ❌ macOS-only | ❌ | Mark it `platforms: [darwin]`. It never appears on Linux or WSL, and Windows Terminal is the host-side analog. |

### How the design handles the gaps

1. **Every catalog entry declares a `kind` (`cli` or `gui`) and a per-platform install block** keyed `darwin`, `linux` and `wsl`. A missing block means "not offered here". `wsl` inherits `linux` by default for CLIs. For GUI apps, `wsl` defaults to a **host target**, described below.
2. **Platform detection** happens in the bootstrap stage. It checks the OS, the architecture, WSL (`WSL_DISTRO_NAME` or `microsoft` in `/proc/version`), whether a display exists (`DISPLAY`/`WAYLAND_DISPLAY`, so headless Linux build boxes skip GUI apps), and the glibc version against the Homebrew support tiers.
3. **A new Plan-screen state, `unavailable-here`,** for items the profile wants but this platform can't install through any configured backend. These rows are shown greyed with the reason and a one-line hint, for example *"Install on the Windows host: `winget install Anthropic.Claude`"*. They never block the run.
4. **Host targets on WSL.** v1 is *report-only*: `sfi` detects whether the Windows-side app exists (through WSL interop, `cmd.exe /c where` or `powershell.exe Get-AppxPackage`/registry) and shows it as present, foreign or missing. It never installs on the host. v2 adds an optional **`winget-host` backend** that calls `winget.exe` from inside WSL, with its own consent step, because it changes the Windows side. Homebrew 6's new `brew bundle` WinGet support is worth evaluating as the mechanism. That keeps "Homebrew everywhere" as the story while still covering the GUI apps.
5. **Linux GUI apps that aren't casks** (VS Code, Claude desktop) are report-only in v1. A later `native-pkg` backend (apt/dnf repo or Flatpak) can be added per distro if the Linux desktop population is large enough to justify it (Q12).

### Platform-specific gotchas

- **Linux bootstrap needs the distro package manager once.** Homebrew on Linux requires `build-essential`/`gcc`, `procps`, `curl`, `file` and `git` first, which means one `sudo apt install …` step (or dnf). `sfi` shows that step in Review like any other command.
- **Prefix differs**: `/opt/homebrew` (Apple Silicon), `/usr/local` (Intel Mac), `/home/linuxbrew/.linuxbrew` (Linux and WSL). Detection templates use `{brew_prefix}`.
- **WSL filesystem.** Keep `sfi` state and Homebrew on the Linux filesystem, never under `/mnt/c`, for performance and permission correctness.
- **Homebrew 6 tap trust.** Third-party taps such as `stablyai/orca` must be **explicitly trusted** before Homebrew evaluates them. `sfi` treats "trust tap X" as its own consented action and records it in the inventory, on every platform.
- **git and sqlite on Linux**: the distro copies (`/usr/bin/git`, `/usr/bin/sqlite3`) count as "present (OS)", just like Apple's on macOS. There is no CLT-shim problem on Linux.

---

## 3. Core concept: ownership states

The central rule is *"only manage what we installed."* Each package on each machine is in exactly one state:

| State | Meaning | What `sfi` may do |
|---|---|---|
| `absent` | Not detected anywhere | Propose install |
| `foreign` | Present, but not installed by `sfi` (developer's own brew install, DMG drag-in, native installer, OS-bundled) | Report only. **Never** upgrade or remove it. Offer **Adopt**. |
| `managed` | Installed by `sfi` and recorded in the inventory | Upgrade, uninstall, repair |
| `adopted` | Was `foreign`, and the developer explicitly handed it to `sfi` | Same as `managed`, but uninstall needs an extra confirmation because the developer originally owned it |
| `drifted` | Recorded as managed, but no longer detected, or detected through a different backend | Flag it. Offer **Reinstall** or **Forget**. |
| `duplicate` | Detected through two or more sources (for example claude-code by brew and natively) | Report which one wins on PATH. Never auto-resolve. |

**Adoption** matters for labs. Without it, a machine where the developer had already installed `gh` through brew can never be brought under fleet version policy. Adoption is opt-in for each package and is recorded with who adopted it and when.

**Self-updating apps** (Claude desktop, VS Code, iTerm2, Copilot, Hermes, native Claude Code): `sfi` owns *presence* and *removal*. Version is delegated to the app's own updater unless the manifest sets `version_policy: pinned`. Pinned is only honored where the backend supports it.

---

## 4. Architecture

```
┌──────────────────────────────── sfi (single binary) ───────────────────────────────┐
│  TUI (Bubble Tea)            CLI / headless (cobra)                                 │
│        │                            │                                               │
│        └──────────┬─────────────────┘                                               │
│                   ▼                                                                 │
│            Planner / Reconciler  ◄── Manifest loader (YAML + JSON Schema, profiles) │
│        (desired vs detected vs inventory → plan of Actions)                         │
│                   │                                                                 │
│      ┌────────────┼──────────────┬───────────────┐                                  │
│      ▼            ▼              ▼               ▼                                  │
│  Detectors     Executor       Inventory       Reporter                              │
│  (PATH, brew,  (runs Actions  (SQLite:        (JSON export,                         │
│   bundles,      via Backends,  packages,       later: push to                       │
│   npm, files)   streams logs)  ops, audit)     central)                             │
│                   │                                                                 │
│     ┌──────┬──────┼───────┬────────┬──────────┐                                     │
│     ▼      ▼      ▼       ▼        ▼          ▼                                     │
│  brew-   brew-   npm    script   dmg/pkg   [choco/winget: v2]                       │
│  formula cask    global (vendor  (direct                                            │
│                         install) download)                                          │
└────────────────────────────────────────────────────────────────────────────────────┘
```

### Backend interface (illustrative only)

```go
type Backend interface {
    ID() string                                  // "brew-cask", "npm", "script", ...
    Available(ctx) (bool, error)                 // is brew/npm present?
    Detect(ctx, Spec) (Detection, error)         // installed? version? provenance path
    Plan(ctx, Spec, Op) ([]Command, error)       // exact commands, shown to user pre-consent
    Apply(ctx, []Command, LogSink) (Result, error)
    Outdated(ctx, Spec) (latest string, err error)
    Capabilities() Caps                          // CanPin, CanUninstall, NeedsSudo, SelfUpdates
}
```

Key choices:

- **Detection is backend-independent and runs first.** Detectors check PATH (`exec.LookPath`, then resolve symlinks to find provenance), `brew info --json=v2 --installed` (one call for everything), app bundles by **bundle ID** through `mdfind "kMDItemCFBundleIdentifier == '…'"` with a fallback scan of `/Applications` and `~/Applications`, and npm with `npm ls -g --json`. Provenance (which path, which backend) is what decides `foreign` versus `managed` versus `duplicate`.
- **Plan, then consent, then apply.** Every mutation becomes a list of concrete `Command`s that the user sees before saying yes. This doubles as `--dry-run`.
- **Script backend guardrails.** Scripts are downloaded to a temp file, never piped `curl | bash`. They are checked against a manifest-pinned SHA-256 where the vendor publishes one (Claude publishes a signed manifest). Otherwise the user sees an "unverified script" warning, and lab policy can forbid unverified scripts. Each script entry declares its own `uninstall` and `detect` steps, because a script install has no package-manager record.
- **Sudo.** Homebrew must never run as root. Some casks run `.pkg` installers that prompt for sudo. The executor hands the terminal to the child process (Bubble Tea `tea.ExecProcess`) for those steps, so the password prompt is real and `sfi` never captures credentials.
- **Idempotent and resumable.** Each action is written to the operations log before and after it runs. A crash mid-run leaves a clear `in_progress` record that the next launch reconciles.

### 4a. Startup bootstrap: Homebrew and npm (J, 2026-11-14)

On every launch, before the Scan screen, `sfi` runs a **bootstrap check**. It does nothing if everything is in place, and it never installs anything the selections don't need.

| Step | Check | If missing: TUI | If missing: `-cl` |
|---|---|---|---|
| 1. Platform | OS, architecture, WSL, display, glibc (§2a) | — | — |
| 2. Xcode CLT (macOS) | `xcode-select -p` | Folded into step 3: the Homebrew installer installs CLT itself through `softwareupdate` | Same, headless through `softwareupdate` |
| 2L. Linux prerequisites | `gcc`/`build-essential`, `procps`, `curl`, `file`, `git` | Prompt, then `sudo apt-get install -y …` (or dnf) | Auto, needs non-interactive sudo |
| 3. **Homebrew** | `brew` on PATH, or at the known prefixes (`/opt/homebrew`, `/usr/local`, `/home/linuxbrew/.linuxbrew`) even when it isn't on PATH | **Prompt**: "Homebrew is required. Install it now?" The full command is shown, and the user may need to type a sudo password. | **Auto**, unless `bootstrap.homebrew: never` |
| 4. Shell environment | `brew shellenv` applied | Set for this process automatically. **Prompt** before adding it to the shell profile. | Set for this process. Profile change controlled by `bootstrap.persist_shellenv` |
| 5. **Node / npm** | `node` ≥ 24.11 and `npm` from any provenance | Checked always. **Prompted only if a selected package uses the npm backend** (today Paperclip) | Auto, if any listed package needs npm |

**How Homebrew is installed.** Download Homebrew's official `install.sh` to a temp file and run it with `NONINTERACTIVE=1`, as the invoking user and never as root. On macOS the script needs **an admin account and sudo**, to create `/opt/homebrew` and install CLT. On Linux it needs sudo to create `/home/linuxbrew`, or it falls back to an unsupported home-directory prefix, which `sfi` refuses.

- **TUI:** the sudo prompt is passed through to the terminal (`tea.ExecProcess`), so the password goes straight to `sudo`.
- **`-cl`:** `sfi` first checks `sudo -n true`. If that fails (no cached credential and no NOPASSWD rule), it **stops immediately with exit code 4** and tells the operator how to fix the image: pre-install Homebrew, or grant the build user NOPASSWD for the bootstrap. It never hangs on a prompt. Bootstrap is the one place where `-cl` may use sudo, and only when `bootstrap.allow_sudo: true` (default `true` for bootstrap, separate from `policy.allow_sudo` for package installs).

**npm comes from the catalog's `node` entry** (`node@24` through brew, §2 row 15). There is no separate npm install. `node@24` is keg-only, so `sfi` links it (`brew link --force node@24`) or adds `$(brew --prefix node@24)/bin` to the managed PATH block. If a developer's nvm, fnm or Volta Node is ≥ 24.11, it satisfies the check as `foreign`, and npm packages are installed into it (Q5 still decides whether that is the long-term rule). Corepack/pnpm for Paperclip is enabled as part of Paperclip's own install.

**Shell environment.** A freshly installed Homebrew is not on PATH, so later steps (and later shells) can't find `brew`, `node` or anything installed with them.
- **Within the run:** `sfi` evaluates `brew shellenv` and applies it to its own environment and to every child process. This always happens.
- **Persisting it:** `sfi` writes a clearly marked block to the user's login profile (`~/.zprofile` for zsh on macOS, `~/.bash_profile` or `~/.profile` on Linux and WSL), shown in Review first:
  ```sh
  # >>> sfi managed: homebrew >>>
  eval "$(/opt/homebrew/bin/brew shellenv)"
  export PATH="$(brew --prefix node@24)/bin:$PATH"
  # <<< sfi managed: homebrew <<<
  ```
  The block is recorded in the inventory, so `sfi` can update or remove exactly those lines and nothing else. In the TUI this is a prompt. With `-cl` it follows `bootstrap.persist_shellenv` (default `true`, because build steps after `sfi` need the tools on PATH).

**Ownership of bootstrap tools.** If `sfi` installs Homebrew or Node, they are recorded as `managed`, with a `bootstrap: true` flag. Homebrew is **never** uninstalled automatically, even by `prune`, because everything else depends on it. A Homebrew that already existed is `foreign` and is simply used.

---

## 5. Manifest format

Use **YAML**, validated against a published **JSON Schema**. YAML is familiar, diffable and reviewable in pull requests, and the schema gives editor autocompletion. Manifests are layered:

- `catalog.yaml` is every package the org knows how to install (the research in §2, encoded).
- `profiles/<factory>.yaml` is the set each software factory needs, with overrides.
- A machine selects one or more profiles. The union is the desired state.

```yaml
# catalog.yaml
apiVersion: sfi/v1
packages:
  claude-code:
    display: Claude Code (CLI)
    category: agent-cli
    detect:
      commands: [claude]
      provenance:                      # how to classify what we find
        native: "~/.local/share/claude/versions/**"
        brew:   "{brew_prefix}/Caskroom/claude-code*/**"
        npm:    "{npm_prefix}/lib/node_modules/@anthropic-ai/claude-code/**"
      version: { cmd: "claude --version", regex: '^(\S+)' }
    kind: cli
    install:
      darwin:
        - backend: brew-cask
          id: claude-code              # stable channel
        - backend: npm                 # fallback if brew unavailable
          id: "@anthropic-ai/claude-code"
      linux:
        - backend: brew-cask           # same cask works on Linux x86_64/ARM64
          id: claude-code
      # wsl: inherits linux (kind: cli)
    version_policy: track              # track | pinned | delegate (self-updater)
    requires: []

  visual-studio-code:
    display: VS Code
    kind: gui
    detect:
      bundle_ids: [com.microsoft.VSCode]
      commands: [code]
    install:
      darwin:
        - backend: brew-cask
          id: visual-studio-code
      linux:
        - backend: report              # no Linux cask; v2: native-pkg
          hint: "Install from Microsoft's apt/rpm repository"
      wsl:
        - backend: host-report         # v2: winget-host
          detect: { winget_id: Microsoft.VisualStudioCode }
          hint: "winget install Microsoft.VisualStudioCode, then add the WSL extension"
    version_policy: delegate

  orca:
    display: Orca (agent IDE)
    detect:
      bundle_ids: [dev.orca.app]       # ⚠ verify real bundle id
      commands: [orca]
    install:
      darwin:
        - backend: brew-cask
          tap: stablyai/orca
          id: stablyai/orca/orca       # never bare "orca" (Plotly)
    version_policy: delegate

  paperclip:
    display: Paperclip (agent orchestration server)
    detect: { commands: [paperclipai] }
    install:
      darwin:
        - backend: npm
          id: paperclipai
          version: "x.y.z"             # pin; avoid @latest in labs
          post_install: ["paperclipai onboard --yes"]   # never as root
      linux:                           # wsl inherits
        - backend: npm
          id: paperclipai
          version: "x.y.z"
    requires: [{ package: node, min_version: "24.11" }]
    service:                           # see §6a
      manager: auto                    # launchd (macOS) | systemd --user (Linux/WSL)
      install_cmd: "paperclipai service install"
      health: { http: "http://127.0.0.1:3100", timeout: 30s }
      options:                         # asked per install, recorded in inventory
        exposure:
          prompt: "Who should be able to reach Paperclip?"
          choices:
            local:  { label: "This machine only (localhost)", default: true }
            remote: { label: "Other machines on the network (enables login)",
                      requires_inputs: [public_base_url, allowed_hostnames],
                      env: { PAPERCLIP_DEPLOYMENT_MODE: authenticated,
                             PAPERCLIP_DEPLOYMENT_EXPOSURE: public } }
          headless: require_explicit   # --set paperclip.exposure=remote; never inferred
      backup_before_upgrade: ["~/.paperclip/instances/default/secrets/master.key", "~/.paperclip/instances/default/db"]
      run_as: invoking-user            # embedded Postgres refuses root

  node:
    display: Node.js (LTS 24)
    kind: cli
    detect:
      commands: [node]
      version: { cmd: "node --version", regex: 'v(\S+)' }
      provenance:
        brew:  "{brew_prefix}/opt/node@24/**"
        nvm:   "~/.nvm/versions/node/**"
        fnm:   "~/.local/state/fnm_multishells/**"
        volta: "~/.volta/**"
    install:
      darwin: [{ backend: brew-formula, id: node@24, link: force }]
      linux:  [{ backend: brew-formula, id: node@24, link: force }]
    satisfies_if: { min_version: "24.11" }   # any provenance counts

  awscli:
    display: AWS CLI v2
    kind: cli
    detect:
      commands: [aws]
      version: { cmd: "aws --version", regex: 'aws-cli/(\S+)' }
      reject_if: { version_lt: "2.0" }       # v1 via pip → duplicate/foreign
    install:
      darwin: [{ backend: brew-formula, id: awscli }]
      linux:  [{ backend: brew-formula, id: awscli }]

  hermes-agent:
    display: Hermes Agent (Nous Research)
    kind: cli
    detect:
      commands: [hermes]
      provenance:
        brew:   "{brew_prefix}/Cellar/hermes-agent/**"
        script: "~/.hermes/hermes-agent/**"
    install:
      darwin:
        - backend: brew-formula
          id: hermes-agent             # NOT "hermes"
      linux:
        - backend: brew-formula
          id: hermes-agent
    version_policy: track              # brew upgrade, not `hermes update`
```

**Defaults.** Every profile entry is either selected by default or `optional: true`. Optional entries appear in the Plan screen **unchecked** and are installed only if the developer ticks them. In headless mode they need `--with <id>`.

**Choice groups.** Some needs can be met by any one of several packages, such as the container runtime. A `choice` entry lists the alternatives. If any of them is already present, the group is satisfied and nothing is proposed. Otherwise the TUI offers a single-select list. The developer can pick none, since the group is off by default.

```yaml
# catalog.yaml (excerpt): a choice group
  container-runtime:
    display: Container runtime
    choice:
      - id: orbstack           # macOS only, light
        platforms: [darwin]
      - id: colima             # formulae colima + docker + docker-compose
        platforms: [darwin, linux]
      - id: docker-desktop     # heavy; licence required at large companies
        platforms: [darwin]
        host_on: [wsl]         # report the Windows-host install
    satisfied_by_any_present: true
```

```yaml
# profiles/agentic-factory.yaml
apiVersion: sfi/v1
profile: agentic-factory
description: Baseline for the agentic software factory
include: [base-dev]                    # profile composition
packages:
  - claude-desktop
  - claude-code
  - copilot-cli
  - github-copilot-app: { optional: true }   # pre-unchecked in the TUI
  - antigravity-cli
  - codex
  - uv
  - jq
  - ripgrep
  - container-runtime: { optional: true }        # choice group, default off
  - session-manager-plugin: { optional: true }
  - gcloud-cli: { optional: true }
  - azure-cli: { optional: true }
  - orca
  - t3-code: { optional: true }
  - paperclip
  - hermes-agent
policy:
  allow_unverified_scripts: false
  allow_adopt: true
```

The catalog and profiles live in **one git repo** owned by the platform team. `sfi` pulls a tagged release of that repo, verifying the signed tag or a checksum, so lab changes go through pull request review. That is also the natural teaching artifact for agentic practice.

---

## 6. State store (inventory)

Use **SQLite**, embedded through a pure-Go driver (`modernc.org/sqlite`, no cgo). It is a single file at `~/Library/Application Support/sfi/state.db` (macOS) or `%LOCALAPPDATA%\sfi\state.db` (Windows).

```sql
packages(        -- one row per package sfi knows about on this machine
  id TEXT PK, state TEXT,              -- managed | adopted | foreign | drifted ...
  backend TEXT, backend_ref TEXT,      -- e.g. brew-cask / stablyai/orca/orca
  installed_version TEXT, install_path TEXT,
  installed_at, installed_by, adopted_at, manifest_rev TEXT)

operations(      -- append-only audit log
  id INTEGER PK, started_at, finished_at, package_id, op TEXT,  -- install/upgrade/uninstall/adopt/forget
  commands_json TEXT, exit_code INT, status TEXT, log_path TEXT, consent_by TEXT)

detections(      -- last scan snapshot, for drift and reporting
  scanned_at, package_id, found BOOL, version, provenance, path)

meta(key PK, value)  -- schema_version, machine_id, last_manifest_rev
```

SQLite fits better than a JSON file because the audit log grows over time and needs transactions for crash safety, and SQL makes fleet roll-up queries trivial. It also needs no extra install on the machine, and the developer can inspect it with the `sqlite3` they already have.

**Source of truth.** The inventory records *intent and ownership*. The package managers record *what is actually on disk*. Every launch reconciles the two, and disagreements show up as `drifted`.

---

## 6a. Managed services (Paperclip)

J's decision is that Paperclip runs as a **service `sfi` manages**, not just a package it installs. A service adds lifecycle on top of the ownership states in §3:

- **Install.** Install the npm package, run `paperclipai onboard --yes` as the developer (never root), then register the service with Paperclip's own `paperclipai service install`. That command should create a **launchd LaunchAgent** on macOS and a **systemd user unit** on Linux (⚠ verify what it actually does on each platform). `sfi` records the unit or plist path in the inventory so it can remove it later.
- **WSL.** systemd must be enabled in `/etc/wsl.conf` (`[boot] systemd=true`). That is the default on current Ubuntu WSL images, but `sfi` checks it and shows the fix if it is off. Paperclip stays reachable from a Windows browser at `localhost:3100` through WSL localhost forwarding.
- **Health.** After every install, upgrade or restart, poll `http://127.0.0.1:3100` until it responds or times out. The Manage tab shows each service as running, stopped or unhealthy, with **start / stop / restart / logs** actions.
- **Network exposure is an install choice (J, 2026-11-14).** The Review screen asks *"Who should be able to reach Paperclip?"*. **This machine only** (localhost) is pre-selected. **Other machines on the network** turns on Paperclip's login mode (`PAPERCLIP_DEPLOYMENT_MODE=authenticated`, `PAPERCLIP_DEPLOYMENT_EXPOSURE=public`) and collects the public base URL and allowed hostnames. `sfi` never picks remote on its own, profiles cannot silently default it, and headless runs need an explicit `--set paperclip.exposure=remote`. On WSL, remote access also needs Windows-side port forwarding or mirrored networking. On macOS the first remote start may trigger the firewall prompt. Both are called out in Review. The choice is stored in the inventory (`services.exposure`), and **Manage → Change exposure** can switch it later with the same consent step.
- **Upgrade.** Before upgrading, copy the `secrets/master.key` and the embedded database directory to `~/Library/Application Support/sfi/backups/<timestamp>/` (or the XDG equivalent). Then stop the service, upgrade, start it, and health-check it. **Roll back** to the previous version and the backup if the health check fails. Paperclip's `npx paperclipai install` path has its own update and rollback, which `sfi` should use if it proves reliable.
- **Uninstall.** Stop the service and unregister it. Remove the package. The `~/.paperclip` data directory is **kept by default**, because it holds the developer's agents, tasks and secrets. Deleting it needs a separate, explicit confirmation.
- **Inventory.** Add a `services` table: `package_id, manager (launchd|systemd-user), unit_path, port, exposure (local|remote), last_health, last_backup_path`.

**Install options in general.** Any package can declare `options:` like the one above. The rules are the same everywhere: the safest choice is the default, a riskier choice needs the developer's explicit selection (or an explicit headless flag), and the chosen value is recorded with the consent in `operations`.

Paperclip is the only service in the list today, but the `service:` block is generic, so later services (local MCP servers, for example) reuse it.

---

## 6b. Seeding tool configuration from files (agents, skills, settings)

*J asked (2026-11-14) whether hermes, Paperclip and Orca can take agents and other configuration from a file at setup time. Research summary:*

| Tool | File-based config? | What can be seeded | How `sfi` would apply it |
|---|---|---|---|
| **Paperclip** | ✅ **Strong.** First-class *company packages* | A folder or `.zip` of markdown plus YAML: `COMPANY.md`, `agents/<name>/AGENTS.md`, `projects/`, `skills/<name>/SKILL.md`, `tasks/`, `.paperclip.yaml` (adapters, budgets, env var *names*) | `paperclipai company import <path\|github-url> [--ref v1.2.0] --target new --new-company-name … [--dry-run]`. Secret **values** are never in the package and must be supplied on the target. Imported agents start with heartbeats **paused**. |
| **Hermes Agent** | ✅ **Good.** Plain files under `~/.hermes` (or `HERMES_HOME`) | `config.yaml` (model, toolsets, `mcp_servers`, with `${VAR}` expansion), `SOUL.md` (agent identity), `skills/`, `cron/`, per-profile state; secrets in `.env` | Copy templated files into `HERMES_HOME`, and/or run `hermes config set KEY VAL` for individual values. Use `hermes -p <profile>` for multiple agents. No import command, but none is needed. |
| **T3 Code** | ⚠ **Settings only, no agent definitions** | A JSON `settings.json` in the state dir (`T3CODE_STATE_DIR`, default `~/.t3/userdata`; ⚠ exact filename unconfirmed in official docs). It holds things like `providerInstances` (which Codex or Claude binaries to use, custom models) and `enableProviderUpdateChecks`. Env vars `T3CODE_PORT`, `T3CODE_HOST`, `T3CODE_AUTH_TOKEN`, `T3CODE_DISABLE_AUTO_UPDATE`. Agents themselves are Claude Code, Codex and others, configured by their own files. | `json_merge` step: merge keys into `settings.json` rather than replacing the file, then set env vars. T3 is beta (0.1.x), so the schema may change, and `sfi` validates and backs up the file before merging. |
| **Orca** | ⚠ **Partial.** Mostly app settings; repo file covers little | `orca.yaml` in a repo (documented key: `worktree.sharedDirectories`), plus `~/.orca/keybindings.json`. Most settings live in the app UI. | **Script the CLI** instead: `orca repo add --path …`, `orca repo set-base-ref …`, `orca automations create …`, `orca skills install …`, `orca serve service install`. There is no documented settings import or export. |

### Design: a `configure` step in saved configurations

A configuration list (§7a) can include an `assets/` folder in the same repo and a per-package `configure:` block that runs **after** install:

```yaml
packages:
  - id: paperclip
    options: { exposure: local }
    configure:
      - type: command
        run: >-
          paperclipai company import ./assets/paperclip/agentic-factory
          --target new --new-company-name "Agentic Factory"
        once: true                       # don't re-import on every run
        requires_env: [ANTHROPIC_API_KEY] # values come from CI secrets or the developer's shell
  - id: hermes-agent
    configure:
      - type: files
        from: ./assets/hermes/            # config.yaml, SOUL.md, skills/
        to: "${HERMES_HOME:-~/.hermes}/"
        template: true                    # ${VAR} expanded at apply time
      - type: env_file
        to: "${HERMES_HOME:-~/.hermes}/.env"
        keys: [OPENROUTER_API_KEY]        # names only; values from the environment
  - id: orca
    configure:
      - type: command
        run: orca skills install <skill>
```

Rules:

- **Secrets never live in git.** Configuration lists declare only env var *names*. Values come from the environment at apply time (CI secrets on build machines, or the developer's shell or keychain interactively). If a value is missing, the run fails with exit code 3 under `-cl`, or prompts in the TUI.
- **Ownership extends to files.** `sfi` records a hash of every file it writes. On later runs, if the developer has edited a file (the hash differs), `sfi` **does not overwrite it**. It flags it as `drifted` and offers a diff in the TUI. With `-cl`, `policy.config_drift: keep | overwrite | fail` decides (default `keep`).
- **Idempotent.** `files` steps compare hashes, `command` steps marked `once: true` are recorded in `operations` and skipped afterwards, and Paperclip imports use `--dry-run` in `sfi apply --dry-run`.
- **Order.** Install, then service start (Paperclip must be running for `company import`), then configure, then health check.
- **Same consent model.** Configure steps appear in Review, with the exact commands and target files, and are covered by the checked-in file under `-cl`.

This replaces the vague "post-install hooks" item in the phase plan.

---

## 7. TUI flow

```
 ┌ Scan ──────────┐   ┌ Plan ──────────────────────────────────────────┐   ┌ Review ──────────┐   ┌ Run ──────────┐   ┌ Summary ┐
 │ bootstrap check│ → │ [x] orca         missing     brew-cask (tap)   │ → │ exact commands   │ → │ per-item      │ → │ result, │
 │ CLT, brew, node│   │ [x] antigravity  missing     brew-cask         │   │ sudo needed: 1   │   │ spinners and  │   │ logs,   │
 │ detectors run  │   │  ·  gh           foreign     brew  [a]dopt?    │   │ unverified: 0    │   │ live logs     │   │ export  │
 │ (parallel)     │   │  ✓  claude-code  managed     ↑ 2.1.299 avail   │   │ [Enter] consent  │   │ (tea.Exec for │   └─────────┘
 └────────────────┘   │  ⚠  claude-code  duplicate   native + brew     │   └──────────────────┘   │  sudo steps)  │
                      └────────────────────────────────────────────────┘                          └───────────────┘
```

- **Plan** is a table grouped by state. Missing required packages are pre-checked, optional ones are not, and foreign ones cannot be selected for install (only for **[a]dopt**). Keys: `space` toggles, `a` adopts, `i` shows info (source, why it is needed, which profile asked for it), `/` filters.
- **Review** is the consent gate. It shows every command, whether it needs sudo, whether a script is unverified, and dependency order. Consent is recorded in `operations.consent_by` with the OS user and timestamp.
- **Manage** is a second tab for `managed` and `adopted` items only: upgrade, uninstall, reinstall, forget. Foreign items never appear here.
- **Save configuration** (`s` on Plan or Review) writes the current answers to a configuration list file (§7a).
- **Headless parity**: `sfi scan`, `sfi plan --profile X`, `sfi apply --profile X`, `sfi apply -cl <file>`, `sfi status --json`, `sfi export`. These matter for build machines, imaging, CI, and teaching demos.

---

## 7a. Saved configurations and unattended runs (`--configuration-list`)

*Requirement from J, 2026-11-14:* run `sfi` on a **build machine** from a **repeatable configuration that is checked into git**, with **no prompts**.

### What a configuration list is

A **configuration list** is a single YAML file that records the *answers* a developer would give in the TUI: which packages, which choice-group pick, which install options, and the ownership policy. The catalog and profiles (§5) say what *can* be installed. A configuration list says what *this kind of machine gets*, and everything is resolved in it, so applying it asks nothing.

```yaml
# configs/build-macos.sfi.yaml  (checked in)
apiVersion: sfi/v1
kind: ConfigurationList
metadata:
  name: build-macos
  description: CI build agents for the agentic factory
  created_by: J                     # filled in by `sfi` on save
  created_at: 2026-11-14T16:50:00Z
catalog:
  source: git+https://github.com/<org>/sfi-catalog.git
  ref: v2026.11.1                   # pinned: same input, same result
  sha256: "…"                       # integrity of the resolved catalog
platforms: [darwin]                 # refuse to apply anywhere else
packages:
  - id: git
  - id: gh
  - id: node
  - id: awscli
  - id: claude-code
    version: "2.1.311"              # optional pin; omitted = catalog's version_policy
  - id: codex
  - id: uv
  - id: jq
  - id: ripgrep
  - id: container-runtime
    choice: colima                  # choice groups must name the pick
  - id: paperclip
    options:
      exposure: local               # risky values (remote) must be written out explicitly
policy:
  foreign: skip                     # skip | adopt | fail. What to do with pre-existing tools
  unavailable_here: warn            # warn | fail. GUI apps on headless or Linux machines
  allow_sudo: false                 # package installs: fail fast instead of hanging on a password prompt
bootstrap:                          # startup checks, §4a
  homebrew: auto                    # auto | never (never = fail if brew is missing)
  node: auto                        # installed only if a listed package needs npm
  allow_sudo: true                  # Homebrew's first install needs sudo; requires `sudo -n` to work
  persist_shellenv: true            # write the marked PATH block to the login profile
  prune: false                      # true: uninstall *managed* items no longer listed
```

### Building and saving one

- **From the TUI.** On the Plan or Review screen, press **`s` (Save configuration)**. `sfi` writes the current selections, the choice picks, the option answers and the policy to a file the developer names (default `./<name>.sfi.yaml`), and pins the catalog revision it used. Nothing is installed. Saving and applying are separate steps, so a developer can build a configuration on a laptop and apply it on a build machine.
- **From the CLI.** `sfi config init --profile agentic-factory -o build.sfi.yaml` writes a starting file with only the profile's defaults. `sfi config export -o mine.sfi.yaml` captures what this machine currently *manages*, which is useful for cloning a known-good box.
- **Validation.** `sfi config validate -cl build.sfi.yaml` checks the file against the JSON Schema and the pinned catalog: unknown IDs, a missing choice pick, an option value the package doesn't offer, platform mismatch. It's meant for a pre-merge CI check on the repo that holds the configs.

### Applying it with no prompts

```bash
sfi apply --configuration-list configs/build-macos.sfi.yaml   # long form
sfi apply -cl configs/build-macos.sfi.yaml                    # J's short form
sfi apply -cl build.sfi.yaml --dry-run                        # print the plan, change nothing
sfi apply -cl build.sfi.yaml --check                          # CI gate: exit 2 if the machine differs
```

- **The filename is required.** `-cl` with no value is a usage error (exit 64). There is no implicit default file, so a bare `sfi apply` never silently applies something.
- **The checked-in file is the consent.** Each operation is recorded as `consent_by = config:<path>@<git-sha or file sha256>`, so the audit log shows exactly which reviewed revision authorized each change. The pull request review on that file is the human approval step.
- **No hidden defaults for risky options.** If a package has an option whose non-default value is risky (Paperclip `exposure: remote`), the risky value is used only when it is **written in the file**. If the option is left out, the safe default (`local`) applies, and `--dry-run` prints it so reviewers can see it. Remote exposure also needs its required inputs (`public_base_url`, `allowed_hostnames`) in the file, or validation fails.
- **Non-interactive by construction.** With `-cl`, `sfi` never opens the TUI and never reads stdin. Anything that would need a person (a sudo password when `sudo -n` isn't available, a missing required option) is a **hard failure with a clear message**, never a hang. Missing Homebrew and Node are installed automatically by the bootstrap (§4a), so a clean build agent only needs an admin user with non-interactive sudo, or Homebrew pre-installed in the image.
- **Existing tools.** `policy.foreign` decides what happens to tools already on the machine: `skip` (default) leaves them alone and counts them as satisfying the requirement, `adopt` takes them under management, and `fail` stops the run, for pristine build images.
- **Exit codes** for pipelines: `0` success or already compliant, `1` an operation failed, `2` drift found (with `--check`), `3` validation error, `4` bootstrap failed (for example, Homebrew missing and no non-interactive sudo), `64` usage error. `--json` writes a machine-readable result (the same schema as `sfi export`) for build logs.
- **Idempotent.** Running the same file twice changes nothing the second time. That is what makes it safe to run at the start of every build.

### Where `-cl` runs: mostly headless Linux CI (J, 2026-11-14)

J expects **CI runs with `-cl` to be mostly Linux machines**, while **macOS machines mostly have an interactive developer**. So the prompt-free path is designed and tested **Linux-first**, and macOS `-cl` is supported but secondary.

Linux CI specifics:

- **Prerequisites for Linuxbrew**, installed by the bootstrap (§4a step 2L) before Homebrew:
  - Debian/Ubuntu: `sudo apt-get update && sudo apt-get install -y build-essential procps curl file git`
  - Fedora/RHEL: `sudo dnf group install -y development-tools && sudo dnf install -y procps-ng curl file git`
  - `sfi` detects the distro from `/etc/os-release`, and fails with exit code 4 on a distro it doesn't know, rather than guessing.
- **Homebrew refuses to run as root.** CI containers often run as root. With `-cl`, `sfi` detects UID 0 and stops with exit code 4 and a clear fix: run as a non-root user with passwordless sudo, or start from Homebrew's official `homebrew/brew` container image. It does **not** create users on its own.
- **glibc.** Prebuilt bottles need glibc ≥ 2.39 (Ubuntu 24.04+). On older runners Homebrew builds its own glibc first, which is slow. `sfi` warns about this in `--dry-run`, so teams can pick a newer base image.
- **No display.** GUI packages and casks resolve to `unavailable-here` and are skipped with a warning (`policy.unavailable_here: warn`), so one configuration list can serve both a developer Mac and a headless runner.
- **Ephemeral runners.** For speed, cache `/home/linuxbrew/.linuxbrew` and `~/.cache/Homebrew` between runs, or bake an image with `sfi apply -cl` run at image-build time and only `--check` at job time.
- **Services in CI.** Paperclip as a systemd user service needs a user session (`loginctl enable-linger <user>`). In containers without systemd, `sfi` reports the service as `unavailable-here` instead of failing, unless the config marks it `required: true`.

#### GitHub Actions hosted runners (checked 2026-11-14)

J asked whether GitHub Actions runners already have Homebrew and npm. **Mostly yes, with one catch for Paperclip:**

| Runner | Homebrew | Node / npm | User |
|---|---|---|---|
| `ubuntu-24.04` / `ubuntu-latest` | ✅ installed (6.0.x) at `/home/linuxbrew/.linuxbrew`, but **not on PATH** | ✅ default **Node 22.x**. Node **24.x is in the tool cache**, not on PATH by default | non-root `runner`, passwordless sudo |
| `macos-*` | ✅ installed and on PATH | ✅ preinstalled | non-root `runner`, passwordless sudo |
| Container jobs (`container:`), self-hosted runners | ❌ usually absent | varies | often root, see above |

What this means for the bootstrap:

- **Look before installing.** Step 3 checks the known Homebrew prefixes even when `brew` isn't on PATH. On Ubuntu runners it finds `/home/linuxbrew/.linuxbrew/bin/brew`, applies its `shellenv`, and installs nothing, so the bootstrap takes seconds.
- **Node 22 doesn't satisfy Paperclip (≥ 24.11).** If a configuration list includes Paperclip, `sfi` installs `node@24` through brew as usual. Alternatively, the workflow can run `actions/setup-node` with `node-version: 24` first, which takes Node 24 from the tool cache and is faster. `sfi` then detects it as a `foreign` Node that satisfies the requirement.
- **Passwordless sudo is available**, so `bootstrap.allow_sudo` works without extra setup, and so do the apt prerequisites.
- **Container jobs and self-hosted runners** fall back to the full bootstrap, including the root check.

```yaml
# .github/workflows/build.yml (excerpt)
jobs:
  build:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4        # optional: only if Paperclip/npm tools are listed
        with: { node-version: 24 }
      - uses: actions/setup-go@v5
        with: { go-version: stable }
      - run: go run github.com/<org>/software-factory-installer/cmd/sfi@v0.3.0 apply -cl configs/build-linux.sfi.yaml --json
```

### The `-cl` flag: a CLI-library caveat

J asked for `-cl` as the short form. Most Go CLI libraries, including `spf13/pflag` (used by cobra), allow **only single-letter shorthands**. To pflag, `-cl` means `-c -l`. Options:

| Option | How | Trade-off |
|---|---|---|
| **A. cobra + argument normalizer (recommended)** | Before cobra parses, rewrite exactly `-cl <file>` and `-cl=<file>` to `--configuration-list`. Also register `-c` as the real single-letter shorthand. | Keeps cobra's subcommands, help and completion. The rewrite is about 15 lines, unit-tested, and documented in `--help`. |
| B. Go stdlib `flag` | Register both `configuration-list` and `cl`. Stdlib `flag` accepts `-cl` and `--cl` natively. | No special-casing, but you lose cobra's subcommand tree and shell completion, or have to rebuild them. |
| C. Use `-c` only | A standard single-letter shorthand. | Doesn't match J's ask. |

**Recommendation: A.** `-cl`, `-c` and `--configuration-list` all work and are documented, and shell completion still offers `--configuration-list`.

### Per-tool setup flags: `--hermes-setup`, `--paperclip-setup`, `--orca-setup` (J, 2026-11-14)

J asked for a flag per tool that applies that tool's setup (agents, skills, settings, §6b) from a file. Each flag **requires a filename**.

| Long | Short | File points to |
|---|---|---|
| `--configuration-list` | `-cl` (also `-c`) | Configuration list (§7a) |
| `--hermes-setup` | `-hs` | Hermes setup spec |
| `--paperclip-setup` | `-ps` | Paperclip setup spec |
| `--orca-setup` | `-os` | Orca setup spec |
| `--t3-setup` | `-ts` | T3 Code setup spec (settings merge only) |

*(J wrote "stepup" and "orcas". I read those as typos for "setup" and "orca". Tell me if you want different spellings.)*

**The setup spec file** is a small YAML file (`kind: ToolSetup`) holding exactly the `configure:` steps from §6b for one tool. Relative paths resolve against the file's own directory, so a spec and its assets can live together in the repo:

```yaml
# setup/paperclip/agentic-factory.setup.yaml
apiVersion: sfi/v1
kind: ToolSetup
tool: paperclip
configure:
  - type: command
    run: paperclipai company import ./company --target new --new-company-name "Agentic Factory"
    once: true
    requires_env: [ANTHROPIC_API_KEY]
```

A new step type, `json_merge`, deep-merges a JSON fragment into an existing settings file (used by T3) and records the merged keys, so `sfi` can later remove only what it added.

As a convenience, `-ps` also accepts a **Paperclip company package** directly (a folder, `.zip`, or `github.com/...` URL), which `sfi` imports with the defaults above. Hermes and Orca take only spec files.

**Behaviour:**

- **Stand-alone.** Each flag runs on its own against a tool that's already installed: `sfi setup -hs setup/hermes/team.setup.yaml`. It works whether the tool is `managed`, `adopted` or `foreign`, because configuring a developer's own Hermes is an explicit request. Only the files `sfi` writes are tracked, and the tool's ownership state doesn't change.
- **Tool not installed.** In the TUI, `sfi` offers to install it first. In non-interactive runs it fails with exit code 3 (*"paperclip is not installed; add it to the configuration list or install it first"*), unless the same run also has a `-cl` that installs it.
- **With `-cl`.** The flags combine: `sfi apply -cl build.sfi.yaml -ps setup/paperclip/prod.setup.yaml`. A per-tool flag **replaces that tool's `configure:` section** in the configuration list for this run. Everything else in the list applies as written. `--dry-run` shows which source was used for each tool.
- **No prompts.** Like `-cl`, a setup flag means the file is the consent, so the run is non-interactive. It is recorded as `consent_by=setup:<path>@<sha>`. Add `--review` to open the TUI's Review screen first instead.
- **Referencing setups from a configuration list.** A package entry may set `setup: <path to a ToolSetup file>` instead of an inline `configure:` block, which is equivalent to passing the flag. That keeps configuration lists short and lets several lists share one setup.
- **Several at once.** `-hs`, `-ps` and `-os` can be used together. They run in the order install, then Paperclip service start, then setups (Hermes, Paperclip, Orca, T3), then health checks. Each flag may appear once.

**Short-flag collisions.** None of the shorthands collide with each other:

- `-hs`, `-ps`, `-os`, `-ts` and `-cl` are all handled by the same exact-token rewrite as `-cl` (Option A).
- `-h` stays help and `-c` stays the configuration list.
- `-o` stays `--output` on `sfi config init/export`, because the rewrite matches only the exact tokens `-os` and `-os=<file>`.
- The single letters `-s`, `-p` and `-t` are deliberately left unused, so `-ps` and `-hs` can never be misread as combined shorthands if the rewrite is ever bypassed.
- A unit test covers the full set.

---

## 7b. File formats: template, validate, and keep them correct (J, 2026-11-14)

Every file a user writes for `sfi` falls into one of two groups:

1. **`sfi`'s own formats**: configuration lists (`kind: ConfigurationList`) and setup specs (`kind: ToolSetup`).
2. **Tool-native files** that a setup spec ships or edits: Hermes `config.yaml` and `SOUL.md`, the Paperclip company package (`COMPANY.md`, `agents/*/AGENTS.md`, `.paperclip.yaml`), the Orca `orca.yaml`, and the T3 Code `settings.json` fragment.

Both groups get the same three things: a **template**, a **validator**, and a **contract rule** that keeps them correct over time.

### Templating: `sfi template`

```bash
sfi template config-list --profile agentic-factory -o configs/build-linux.sfi.yaml
sfi template hermes    -o setup/hermes/team.setup.yaml      # also writes assets/: config.yaml, SOUL.md
sfi template paperclip -o setup/paperclip/factory.setup.yaml # also writes company/ skeleton
sfi template orca      -o setup/orca/repos.setup.yaml
sfi template t3        -o setup/t3/providers.setup.yaml       # also writes settings.fragment.json
```

- The output is **annotated**: every field has a comment explaining it, its allowed values, and its default, and optional sections are commented out.
- Each file starts with a `# yaml-language-server: $schema=<url>` header, so the VS Code YAML extension gives autocomplete and inline errors as soon as the file is opened.
- Templates are generated from the **same schemas the validator uses** and from the pinned tool version in the catalog, so a template can never describe a format the validator rejects.
- `--minimal` writes the file without comments, and `--from-machine` pre-fills it from the current machine (for example, the Hermes config already in `~/.hermes`). Secret values are never copied, only their names.

### Validation: `sfi validate`

```bash
sfi validate configs/build-linux.sfi.yaml          # kind detected from the file
sfi validate setup/**/*.setup.yaml --deep          # also run the tool's own checker, if the tool is installed
sfi validate configs/ setup/ --format github        # GitHub Actions annotations on the PR diff
```

What it checks, in order:

1. **Syntax**: YAML or JSON parse errors, with `file:line:col`.
2. **Schema**: the file against its JSON Schema (`sfi` kinds and tool-native files), reported with line numbers and the offending key. For example, `team.setup.yaml:14:5 configure[1].type: "file" is not one of files, env_file, command, json_merge`.
3. **Semantics**: package IDs exist in the pinned catalog, choice groups name a valid pick, option values are allowed, `-cl` risky options have their required inputs, referenced asset paths exist, required env var *names* are declared, and no secret-looking values (API key patterns, `.env` contents) are committed. That last check is an error, not a warning.
4. **Version fit**: each tool-native file declares the tool version range it was written for. If the catalog pins a version outside that range, the result is a warning, or an error under `--strict`.
5. **`--deep` (optional)**: run the tool's own checker when it's installed. That means `paperclipai company import <pkg> --dry-run`, `hermes config check` against a temp `HERMES_HOME`, and for T3, a parse plus a key check against the pinned schema.

Exit codes: `0` valid, `3` invalid (the same as validation errors in `apply`). `--json` writes machine-readable results.

**Validation is automatic.** `sfi apply -cl` and every `sfi setup -xs` validate first and refuse to run on errors. In the TUI, the Review screen shows validation results, and a **Files** tab lists the configuration and setup files found in the current repo with their status. From there, `e` opens a file in `$EDITOR` and it is revalidated on save, `t` creates a new one from a template, and `d` shows the diff against what is currently applied.

### The rule: verify formats on every new build (contract tests)

> **Rule: no `sfi` release and no catalog version bump ships unless every format contract passes against the exact tool versions it pins.**

How it works:

- **Pinned schemas.** `schemas/` in the repo holds one JSON Schema per format and version: `schemas/sfi/configuration-list.v1.json`, `schemas/sfi/tool-setup.v1.json`, `schemas/hermes/config.0.22.json`, `schemas/paperclip/company-package.<ver>.json`, `schemas/t3/settings.0.1.json`, `schemas/orca/orca-yaml.<ver>.json`. The catalog entry for each tool names the schema version that matches its pinned tool version.
- **Golden fixtures.** `testdata/contracts/<tool>/` holds known-good and known-bad example files, including every template output.
- **Contract tests in CI**, run on every pull request that touches the catalog, schemas, templates or the `sfi` code, and nightly against each tool's latest release:
  1. Validate all fixtures and all template outputs against their schemas. Good ones must pass and bad ones must fail.
  2. In a Linux container (plus a macOS runner for macOS-only tools), install the **pinned** tool version through `sfi` itself, apply the golden setup, and run the tool's own checker (`--deep`). Then confirm the tool actually starts with that config, for example by health-checking Paperclip and making sure the imported agents are listed.
  3. **The nightly job against latest releases** is the early warning for vendor format drift. T3 (beta) and Hermes are the most likely to change. A failure there opens an issue. It doesn't block, because pinned versions still pass. A failure in the pinned job blocks the merge.
- **Bumping a tool version** in the catalog requires the contract job to pass. If the vendor changed the format, the same pull request must add a new schema version and update the templates, so the format change becomes a reviewed diff rather than a surprise on a build machine.
- **Runtime guard.** At apply time, if the *installed* tool version (for example a `foreign` Hermes the developer upgraded on their own) is outside the schema's range, `sfi` warns in the TUI. Under `-cl` it fails, unless the configuration list sets `policy.format_mismatch: warn`.

Gaps: Orca and T3 don't publish formal schemas, so `sfi` maintains its own, derived from their docs and from observed files. Those schemas are marked `vendor-unofficial`, and their contract tests rely more on "the app starts and reads the settings" than on a vendor validator.

User-facing instructions for each file live in [`user_docs/`](../user_docs/README.md).

---

## 8. Language and framework recommendation

**Recommendation: Go + Bubble Tea (charmbracelet: bubbletea, bubbles, lipgloss, huh for forms).**

| Criterion | Go + Bubble Tea | Python + Textual | Rust + Ratatui |
|---|---|---|---|
| Runs on a fresh Mac with nothing installed | ✅ single static binary | ❌ needs a Python runtime, which macOS no longer ships usefully; bundling with PyInstaller is fragile | ✅ static binary |
| Bootstrapping chicken-and-egg (installer that needs an installer) | none | real problem | none |
| Cross-compile to Windows later | ✅ trivial (`GOOS=windows`) | ok | ✅ |
| Shelling out to brew, npm, scripts with streamed output | ✅ excellent `os/exec`, `tea.ExecProcess` for interactive sudo | ✅ | ✅ |
| TUI maturity and look | ✅ very mature, used by gh-dash, soft-serve and others | ✅ richest widgets | good, more manual |
| Embedded SQLite without cgo | ✅ `modernc.org/sqlite` | ✅ stdlib `sqlite3` | ✅ `rusqlite` (bundled C) |
| Code signing and notarization for MDM distribution | ✅ simple | painful | ✅ simple |
| Enterprise developer and agent familiarity | high | highest | lower |

The deciding factor is that **the tool's job is to set up a machine that may have nothing on it**. A Go binary can be dropped by Jamf or curl and run with no runtime. Rust is equally viable, but it is slower to iterate on and fewer enterprise developers can maintain it. Textual has the nicest widgets, but it would make Python a prerequisite of the very tool that installs prerequisites.

Distribution: a signed and notarized universal macOS binary on GitHub Releases, plus a Homebrew tap (`brew install <org>/tap/sfi`) for developers who already have brew, plus an MDM package for lab images.

---

## 9. Fleet and lab considerations

1. **Single-user machines (decision, J 2026-11-14).** Lab Macs are usually used by one developer, so the design assumes **the developer owns Homebrew and runs `sfi` as themselves**. There is no admin-user indirection. If `sfi` finds the brew prefix owned by a different user, it stops and explains why rather than trying to work around it. Multi-user support (a dedicated brew-owner account) stays a possible later extension.
2. **Per-user versus per-machine installs.** Casks go to `/Applications` (machine-wide). Native Claude Code, script-installed Hermes and npm-global with a user prefix go into `$HOME` (per-user). The inventory must record the scope, and "installed" must be evaluated per user for per-user items.
3. **Managed settings.** Claude Code supports managed settings (channel, minimum and maximum version), and Copilot is governed by org policies. `sfi` should *check* these and report them rather than set them in v1.
4. **Licensing and auth gates.** Copilot (seat plus CLI and app policies), Claude (Pro/Max/Team/Enterprise or API), and Antigravity (Google account or plan) all install fine but don't work without entitlement. The Summary screen should show "installed, sign-in required" next steps.
5. **Network.** Lab proxies and TLS interception affect brew, npm and vendor scripts differently. Support `HTTPS_PROXY` and custom CA passthrough, and allow mirror overrides (an internal npm registry, Homebrew bottle mirror) in the manifest.
6. **Reporting.** v1: `sfi export --json` writes a machine report (detections plus inventory plus manifest revision) that an MDM extension attribute or a nightly job can collect. v2: optional push to a small collector, or reuse an MDM inventory.
7. **Unattended mode.** For imaging and build machines, `sfi apply -cl <file>` (§7a). The checked-in configuration list is the consent, recorded as `consent_by=config:<path>@<sha>`, so the audit trail stays honest.
8. **Upgrades of `sfi` itself.** It updates itself from signed releases, or through its own Homebrew tap. The manifest declares `min_sfi_version`.

---

## 10. Windows notes

- **Windows = WSL 2 + Homebrew.** `sfi` runs *inside* the WSL distro as a Linux binary. All CLIs (claude, copilot, gh, git, tmux, node, agy, hermes, paperclip) install there through Homebrew or npm, exactly as on Linux.
- **GUI apps live on the Windows host.** Claude desktop, VS Code, the Copilot app, T3 Code and Orca. v1 reports them (present, foreign or missing, plus a `winget` hint). v2 adds the optional `winget-host` backend driven from WSL. **WinGet beats Chocolatey** for this job: Anthropic, GitHub and Microsoft all publish WinGet IDs, and Homebrew 6's `brew bundle` speaks WinGet.
- A separate native-Windows `sfi.exe` (no WSL) is out of scope unless a factory needs native Windows toolchains. The `Backend` interface would still support it with no changes.
- iTerm2 has no Windows build (Windows Terminal is the analog). tmux exists only inside WSL, which is fine.

---

## 11. Phased plan

| Phase | Scope |
|---|---|
| **0. Spike** | Detectors only (`sfi scan --json`) run against 3–5 real lab Macs to validate provenance rules and find surprises (duplicates, nvm Node, Apple git). |
| **1. MVP** | Startup bootstrap (§4a: CLT, Homebrew, shellenv, Node/npm), brew-formula and brew-cask backends, YAML catalog and profiles, SQLite inventory, TUI Scan → Plan → Review → Run → Summary, install only. |
| **2. Manage** | Upgrade, uninstall, adopt, forget, drift; npm backend and the **service manager** (unlocks paperclip, §6a); headless parity. |
| **2b. Config lists + Linux CI** | Save configuration from the TUI, `sfi config init/export/validate`, `apply -cl` with no prompts, `--check`/`--dry-run`, exit codes. **The Linux headless bootstrap lands here** (distro prerequisites, Linuxbrew, the root check), because `-cl` will mostly run on Linux CI. Pulled forward for J's build-machine use case. |
| **3. Fleet** | Signed manifest repo pull, JSON export, MDM packaging. |
| **4. Linux desktop + WSL** | Interactive Linux and WSL TUI polish, WSL host detection (report-only). The Linux headless bootstrap already arrived in 2b. Mostly free, because the brew and npm backends are shared. |
| **4b. Host GUI apps** | Optional `winget-host` backend from WSL. Optional `native-pkg` (apt/dnf or Flatpak) for Linux GUI apps that aren't casks. |
| **2c. Formats** | `sfi template`, `sfi validate` (syntax, schema, semantics, version fit, `--deep`), the TUI Files tab, published schemas, and the contract-test CI rule (§7b). Lands with or right after config lists, because users write these files first. |
| **5. Configure** | `configure:` steps (§6b): Paperclip company import, Hermes config files and profiles, Orca CLI scripting, then MCP servers and managed settings. Could move earlier if J's factories need seeded agents from day one. |

---

## 12. Open questions for J

1. ~~Gemini CLI vs Antigravity CLI~~ **Answered:** use Antigravity CLI.
2. **Confirm identities.** "hermes" = Nous Research **Hermes Agent**? "paperclip" = **paperclipai/paperclip**? "orca" = Stably AI **Orca**? "T3 desktop" = **T3 Code** (not T3 Chat)? For Hermes, the CLI, the desktop app, or both?
3. ~~Paperclip as a service~~ **Answered:** run it as a managed service (§6a). Exposure is chosen at install time: localhost by default, and remote (with login) only on explicit opt-in.
4. **Minimum versions.** Is Apple's bundled git and sqlite acceptable, or do factories require minimum versions, which would mean installing brew copies alongside?
5. **Node managers.** If a developer uses nvm, fnm, asdf or Volta, should `sfi` install npm packages into *their* Node, or install a brew `node` it owns?
6. ~~Machine model~~ **Answered:** usually single-user, so the developer owns Homebrew (§9.1).
7. **Claude Code channel.** Is the brew `claude-code` (stable, upgraded by `sfi`) the standard, or do you prefer native auto-update with managed-settings version floors?
8. **Adoption default.** When `gh` and others are already brew-installed by the developer, offer adoption interactively (current design), or auto-adopt in lab policy mode?
9. **"Multiple software factories."** Can one machine belong to several factories at once (profile union), and can two factories conflict, for example by pinning different versions of the same tool?
10. **Distribution.** Is Jamf (or another MDM) available in the lab to push `sfi`, or is it a self-serve `brew install`/curl?
11. **WSL host apps.** On Windows/WSL machines, is report-only acceptable for GUI apps in v1, or should `sfi` install them on the Windows host through WinGet from the start? That means a second consent scope, and possibly admin rights on Windows.
12. **Linux desktops.** Are there Linux desktop machines (not just headless boxes) in the lab? If so, VS Code and Claude desktop would need an apt/dnf or Flatpak backend, because Homebrew has no Linux casks for them.
13. **Config lists and the catalog.** Should a configuration list pin the catalog by git tag (current design), or embed a full snapshot of the catalog entries it uses so it is completely self-contained?

## 13. Install and usage (draft README content)

*The module path is a placeholder: `github.com/<org>/software-factory-installer`. Replace it once the repo exists.*

### Prerequisites

- macOS 13+ (primary), Linux (Ubuntu 24.04+ or another distro with glibc ≥ 2.39), or Windows with WSL 2 + Ubuntu.
- **Go 1.24+**, but only if you run from source. Release binaries need nothing.
- Homebrew is **not** required beforehand. `sfi` offers to install it (with consent) on first run.

### Option 1: run straight from the repo (no install)

```bash
git clone https://github.com/<org>/software-factory-installer.git
cd software-factory-installer
go run ./cmd/sfi                          # opens the TUI
go run ./cmd/sfi scan                     # read-only: what's present, missing, managed
go run ./cmd/sfi apply -cl configs/build-macos.sfi.yaml --dry-run
```

`go run` compiles into a temporary cache on every invocation, so it is ideal for trying the tool, for contributors, and for teaching sessions. The repo will carry sample configs under `configs/` and the default catalog under `catalog/`, so `go run` works offline against the checked-in catalog (`--catalog ./catalog`).

### Option 2: install from source with Go

```bash
go install github.com/<org>/software-factory-installer/cmd/sfi@latest     # or @v0.3.0 to pin
sfi --version
```

This puts the binary in `$(go env GOBIN)`, or `$(go env GOPATH)/bin` (usually `~/go/bin`). Add that directory to `PATH` if `sfi` isn't found. For a build machine, **pin a tag** rather than `@latest`, so the tool version is as repeatable as the configuration list.

### Option 3: build a binary from the repo

```bash
make build            # → ./bin/sfi, with version stamped through -ldflags
make build-all        # darwin/arm64, darwin/amd64, linux/amd64, linux/arm64
./bin/sfi
```

The binary is pure Go (no cgo, because SQLite comes from `modernc.org/sqlite`), so cross-compiling needs no extra toolchains.

### Option 4: release binaries and Homebrew tap (later)

```bash
brew install <org>/tap/sfi
# or download a signed, notarized binary from GitHub Releases
```

### Everyday usage

```bash
sfi                                   # TUI: Scan → Plan → Review → Run → Summary
sfi scan [--json]                     # detect only, no changes
sfi plan --profile agentic-factory    # show what would be installed
sfi apply --profile agentic-factory   # interactive consent in the terminal
sfi status [--json]                   # managed, adopted and foreign items, drift
sfi upgrade [<id>…]                   # managed and adopted items only
sfi uninstall <id>                    # managed and adopted items only, with confirmation
sfi adopt <id>                        # take over a pre-existing tool
sfi service status|start|stop|logs paperclip
sfi export --json > report.json       # machine report for fleet roll-up
sfi setup -hs setup/hermes/team.setup.yaml          # configure an installed Hermes
sfi setup -ps setup/paperclip/agentic-factory.setup.yaml
sfi setup -os setup/orca/repos.setup.yaml
sfi setup -ts setup/t3/providers.setup.yaml
sfi apply -cl configs/build-linux.sfi.yaml -ps setup/paperclip/ci.setup.yaml   # override one tool's setup
```

### Build-machine usage (no prompts)

```bash
# one-time: save a configuration from the TUI (press `s`), or
sfi config init --profile agentic-factory -o configs/build-macos.sfi.yaml
git add configs/build-macos.sfi.yaml && git commit -m "Build agent toolset"

# in the pipeline or image build:
go run github.com/<org>/software-factory-installer/cmd/sfi@v0.3.0 \
  apply -cl configs/build-macos.sfi.yaml --json
# or, if sfi is already on the image:
sfi apply -cl configs/build-macos.sfi.yaml --check   # fail the build if the agent drifted
```

`go run <module>@<version>` (Go 1.17+) builds and runs a pinned release without cloning or installing, which suits ephemeral CI runners that already have Go.

---

## Sources

- Hermes Agent install: https://hermes-agent.nousresearch.com/docs/getting-started/installation · formula: https://formulae.brew.sh/formula/hermes-agent
- Paperclip export/import: https://docs.paperclip.ing/guides/power/export-import · Hermes configuration: https://hermes-agent.nousresearch.com/docs/user-guide/configuration/ · Orca settings and CLI: https://orca.dev/docs/settings · https://orca.dev/docs/cli/reference
- Paperclip: https://paperclip.ing/llms.txt · https://docs.paperclip.ing/guides/getting-started/installation · https://github.com/paperclipai/paperclip
- Homebrew on Linux and WSL: https://docs.brew.sh/Support-Tiers · https://brew.sh/2026/06/11/homebrew-6.0.0/ · per-cask platform tables on formulae.brew.sh
- Orca: https://github.com/stablyai/orca
- T3 Code cask: https://formulae.brew.sh/cask/t3-code
- Claude Code setup: https://code.claude.com/docs/en/setup · Claude cask: `claude` (Homebrew cask source)
- Copilot CLI install: https://docs.github.com/en/copilot/how-tos/copilot-cli/install-copilot-cli · https://formulae.brew.sh/cask/copilot-cli
- GitHub Copilot app: https://docs.github.com/en/copilot/concepts/agents/github-copilot-app · https://github.blog/changelog/2026-06-17-github-copilot-app-generally-available · https://formulae.brew.sh/cask/github-copilot-app
- Gemini CLI: https://www.geminicli.com/docs/get-started/installation · https://formulae.brew.sh/formula/gemini-cli · https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/
