# Getting Started

This guide takes you from a clone of the repository to a machine with the agentic-factory toolset installed. It covers Increment 1 of `facman`, the **Agentic Software Factory Manager**: the TUI and the headless commands `scan`, `plan`, `apply`, `status` and `doctor`.

`facman` works the same way everywhere. It **detects** what is already installed and how it got there. It **proposes** only what is missing. It **asks** before it changes anything, and it then **manages only what it installed**. Nothing on your machine changes until you consent.

## 1. Before you start

| You need | Notes |
|---|---|
| macOS (Apple silicon or Intel), Linux (amd64 or arm64), or WSL 2 | macOS is the primary target. On Linux, GUI apps (casks) are offered only when a display is present. On WSL, Windows GUI apps are reported with their `winget` command for you to run on the Windows side. |
| A normal user account | `facman` refuses to run as root (exit 4), as Homebrew does. It asks for `sudo` itself only for steps that need it, by handing the terminal to that command. |
| Homebrew (optional) | If Homebrew is missing, `apply` and the TUI offer to install it as the first step of the plan. On Linux, Homebrew is found at `/home/linuxbrew/.linuxbrew` even when it is not on `PATH`. |
| Go 1.27 or later | Only for building from source, which is the way to get `facman` until the first tagged release. |
| Xcode Command Line Tools (macOS) | Homebrew needs them. `facman` reports when they are missing. |

## 2. Get `facman`

Build from source. `make build` produces one static binary with no runtime dependencies:

```sh
git clone https://github.com/stainedhead/software-factory-manager.git
cd software-factory-manager
make build
bin/facman --version      # facman <version> (Agentic Software Factory Manager)
```

Copy `bin/facman` anywhere on your `PATH`, for example `~/.local/bin` or `/usr/local/bin`. To build for every supported platform at once, run `make build-all`. It writes `bin/<os>-<arch>/facman` for darwin and linux, amd64 and arm64.

You can also run straight from the source tree without building:

```sh
go run ./cmd/facman scan
```

Or install into `$(go env GOPATH)/bin`:

```sh
go install github.com/stainedhead/software-factory-manager/cmd/facman@latest
```

Binaries built with `go run` and `go install` report the version `dev`. Tagged releases will publish binaries and checksums on GitHub Releases. None has been cut yet.

## 3. First scan: see what you already have

`scan` is read-only. It checks every package in the built-in catalog and reports its **ownership state** and **provenance**, meaning how it got there:

```console
$ facman scan
Machine:  darwin/arm64
Homebrew: /opt/homebrew/bin/brew

PACKAGE                 STATE             PROVENANCE  VERSION  PATH
claude-code             absent            -           -        -
gh                      foreign           brew        2.62.0   /opt/homebrew/bin/gh
node                    foreign           nvm         22.11.0  /Users/you/.nvm/versions/node/v22.11.0/bin/node
paperclip               unavailable-here  -           -        -
…
  paperclip: needs the npm backend (Increment 2)
```

| State | Meaning |
|---|---|
| `absent` | Not installed. `facman` can propose it. |
| `managed` | Installed by `facman`. It is recorded with the consent that allowed it. |
| `adopted` | Installed by someone else and taken over by you. Adopting arrives in Increment 2. |
| `foreign` | Installed some other way (your own `brew install`, nvm, a `.dmg`, the OS). `facman` never touches it. |
| `drifted` | Recorded as managed, but no longer where `facman` put it. |
| `duplicate` | Installed more than once. The scan says which copy wins on `PATH`. |
| `conflict` | Files that `facman` does not own sit where the install would write. `facman` refuses to install over them. |
| `unavailable-here` | Not installable on this platform, or not until a later increment. The reason is shown. |

Use `facman scan --json` for a machine-readable report (schema v1). The [usage examples](usage-examples.md#script-with-json) show how to query it.

## 4. Plan: see exactly what would run

`plan` is read-only too. It needs at least one profile. The built-in profiles are `agentic-factory` (the default for the TUI, which includes `base-dev`) and `base-dev` (developer basics).

```console
$ facman plan --profile agentic-factory
Plan (profiles: agentic-factory), 20 command(s) in order:
   1. claude-code        brew install --cask claude-code
   …
   7. orca               brew tap stablyai/orca
   8. orca               brew trust --cask stablyai/orca/orca   [after 7]
   9. orca               brew install --cask stablyai/orca/orca   [may ask for sudo; after 8]
  10. node               brew install node@24
  …

Not installed:
  paperclip: unavailable-here; needs the npm backend (Increment 2)
```

Every line is the exact command `facman` would run, in order, with dependencies first. Optional packages, such as `gcloud-cli`, `azure-cli` or a container runtime, are left out until you ask for them with `--with` (for example `--with gcloud-cli`, or `--with colima` to pick a member of the `container-runtime` choice group). Anything already present in any form is not planned again.

## 5. Install: the TUI or `apply`

### The TUI (recommended on a workstation)

Run `facman` with no command in a terminal:

```sh
facman                               # profile agentic-factory
facman --profile base-dev            # another profile
facman --with gcloud-cli             # pre-tick an optional package
```

The TUI walks through five screens:

| Screen | What you do |
|---|---|
| **Scan** | Read what was found: machine, Homebrew, Node, counts by state, and findings. Press `Enter`. |
| **Plan** | Packages grouped by state. Missing required packages start ticked. Use `space` to tick or untick, `i` for details (source, which profile wants it, detections, conflict evidence), and `/` to filter. Press `Enter`. |
| **Review** | Every exact command, in order, with the sudo, tap, trust and "unverified script" marks. **`Enter` here is your consent and the only key that gives it.** `Esc` goes back. |
| **Run** | Progress for each command. A command that needs `sudo` gets the terminal so you can type your password into it directly. `Ctrl+C` asks, then stops after the current command. Commands are never killed midway. |
| **Summary** | What was installed, which tools need a sign-in, any line to add to your shell profile, and where the logs are. Press `q` to quit. |

`?` shows help on every screen. `q` quits with nothing changed at any point before Review. When stdout is not a terminal (for example `facman | cat`), `facman` prints its about text and exits 0 instead of opening the TUI.

### Headless `apply`

`apply` prints the same plan and asks once on the terminal:

```console
$ facman apply --profile base-dev
Plan (profiles: base-dev), 8 command(s) in order:
   1. git                brew install git
   …
   8. awscli             brew install awscli

Proceed? [y/N] y
==> [1] git: brew install git
…
Summary:
  git: installed
  …
  Node.js (LTS 24): installed

Add to your shell profile:
  export PATH="/opt/homebrew/opt/node@24/bin:$PATH"
```

The default is No. An empty line, `n`, end of input or `Ctrl+C` at the prompt, or `Ctrl+C` while the scan or plan is still running, changes nothing and exits 0. There is no `--yes`. `apply` needs a terminal on stdin and exits 64 otherwise. Prompt-free runs for build machines arrive with [configuration lists (`-cl`)](configuration-list.md) in Increment 3. Use `facman apply --profile base-dev --dry-run` to print the plan anywhere, change nothing, and skip the prompt.

Run the same `apply` again and it finds nothing to do (`Nothing to install.`, exit 0). If a run was interrupted, the next one finishes the remaining work.

## 6. Afterwards: status and doctor

```sh
facman status     # what facman manages, plus foreign, drifted and duplicate packages
facman doctor     # link conflicts and untrusted taps across the whole catalog; exit 2 if something needs fixing
```

Every install is written to the audit log in the state database, along with who consented and when. Each command's output goes to its own log file. See [Configuration, state and logs](configuration.md#state-and-logs) for where those live.

## 7. Next steps

- [Usage examples](usage-examples.md): scripting with `--json`, optional packages, custom catalogs, conflicts, the Homebrew bootstrap, and Linux and WSL.
- [Configuration](configuration.md): every flag, environment variable, path and exit code.
- Sign in to the tools the Summary lists (for example `claude`, `codex`, `copilot`). `facman` never handles credentials.

Back to [user-docs](README.md).
