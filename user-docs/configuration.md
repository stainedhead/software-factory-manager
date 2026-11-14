# Configuration

This is the reference for what `facman` Increment 1 accepts: commands, flags, catalogs and profiles, environment variables, state paths and exit codes. Anything marked **Later** is designed but not implemented yet. Using one of those flags or commands today is a usage error (exit 64), and the message names the increment it arrives in.

For a walk-through, see [Getting started](getting-started.md). For worked tasks, see [Usage examples](usage-examples.md).

## Commands

| Command | Reads | Writes | Exit codes |
|---|---|---|---|
| `facman` | Machine, catalog, state | Only after `Enter` on Review | 0, 1, 3, 4, 64 |
| `facman scan` | Machine, catalog, state | Detections in the state DB (skipped while another run holds the lock) | 0, 3, 4, 64 |
| `facman plan --profile P` | Machine, catalog, state | Detections in the state DB | 0, 3, 4, 64 |
| `facman apply --profile P` | Machine, catalog, state | Installs, `operations` and `packages` rows, logs (after `y`) | 0, 1, 3, 4, 64 |
| `facman status` | State, machine | Nothing | 0, 3, 4, 64 |
| `facman doctor` | Machine, whole catalog | Nothing | 0, 2, 3, 4, 64 |
| `facman --version`, `--about`, `--help` | Nothing | Nothing | 0 |

**Later.** `upgrade`, `uninstall`, `adopt`, `forget`, `restore` and `service` arrive in Increment 2. `config` arrives in Increment 3. `template` and `validate` arrive in Increment 4. `setup` and `export` arrive in Increment 5.

## Flags

| Flag | Commands | Meaning |
|---|---|---|
| `--profile <name>` | `plan`, `apply` (required); `facman` (default `agentic-factory`) | The profile to install. Repeat the flag to want the union of several profiles. Required beats optional, and conflicting version pins exit 3, naming both profiles. An empty value exits 64. An unknown name exits 3 and lists the known ones. |
| `--with <id>` | `plan`, `apply`, `facman` | Include an optional package, for example `--with gcloud-cli`. For a choice group, name the member: `--with colima`, `--with orbstack` or `--with docker-desktop` for `container-runtime`. `--with container-runtime` alone, or two members of one group, exits 3. `--with` on a package that is already required is accepted and has no effect. Repeatable. |
| `--catalog <dir>` | every command and `facman` | Use the catalog in `<dir>` instead of the built-in one. See [Catalog directories](#catalog-directories). |
| `--dry-run` | `plan`, `apply` | Print the plan and change nothing. `apply --dry-run` needs no terminal and asks nothing. |
| `--json` | `scan`, `plan`, `apply`, `status`, `doctor` | Write a report (schema v1) to stdout. With `apply`, the command list and the prompt go to stderr, so stdout stays machine-readable. |
| `--version` | none | Print `facman <version> (Agentic Software Factory Manager)`. |
| `--about` | none | Print the application name, purpose, version and repository. |
| `-h`, `--help` | every command | Help. |

Repeated values for the same flag are de-duplicated. An unknown flag, a value flag with no value, or an unknown command exits 64.

**Later.** All of these exit 64 today with the increment named:

| Flag | Arrives in |
|---|---|
| `--configuration-list` / `-cl <file>`, `--check`, `--set` | Increment 3 ([configuration lists](configuration-list.md)) |
| `--no-paperclip` / `-nop` | Increment 2, with the npm backend |
| `--hermes-setup` / `-hs`, `--paperclip-setup` / `-ps`, `--orca-setup` / `-os`, `--t3-setup` / `-ts`, `--review` | Increment 5 ([per-tool setup](README.md)) |
| `--catalog <git URL>` | A later increment. Today only a local directory works. |

The short forms are recognized only as whole words. `-nop=x` and `-no` are usage errors.

## Profiles

| Profile | Contents |
|---|---|
| `agentic-factory` | Includes `base-dev`. Adds Claude Code, Claude Desktop, Copilot CLI, Antigravity CLI, Codex, Hermes Agent, Orca, Paperclip, uv, jq and ripgrep. Optional: GitHub Copilot app, a container runtime (OrbStack, Colima or Docker Desktop), Session Manager plugin, gcloud CLI, Azure CLI and T3 Code. |
| `base-dev` | git, GitHub CLI, tmux, Visual Studio Code, iTerm2 (macOS only), sqlite, Node.js 24 and AWS CLI v2. |

Paperclip is in the profile but is `unavailable-here` until the npm backend ships in Increment 2. Orca's Linux support is unverified, so on Linux it is reported, not installed. Node is installed as the keg-only `node@24`. `facman` never runs `brew link --force`, so the Summary prints the `PATH` line to add to your shell profile.

## Catalog directories

`--catalog <dir>` takes a directory in the same layout as the built-in catalog:

```
<dir>/
  catalog.yaml            # apiVersion: facman/v1, packages: {<id>: {display, kind, detect, install, requires, …}}
  profiles/
    <name>.yaml           # apiVersion: facman/v1, profile: <name>, description, include, packages, policy
```

- Each profile file must be named after its `profile:` value. Only `*.yaml` files in `profiles/` are read.
- Profile `packages` entries are a bare ID or `{<id>: {optional: true, version: "x"}}`.
- Decoding is strict. An unknown key, a wrong type, an unknown backend, a `requires` cycle or a profile that names an unknown package is exit 3, with every problem listed as `file:line:col`. Unknown keys are tolerated only under `service:` and `policy:`, which later increments use.
- Values are checked against strict allow-lists, because a catalog is input that can come from another factory. Breaking one is exit 3, naming the field:
  - package IDs and profile names: lowercase letters, digits and `. _ + @ -`, starting with a letter or digit (`node`, `docker-compose`);
  - brew `id`: `name` or `user/repo/name` (letters, digits and `. _ + @ -`; never starting with `-`), and a step with a `tap` must name an item of that tap (`stablyai/orca/orca` for tap `stablyai/orca`);
  - `tap`: `user/repo`;
  - `detect.commands`: bare executable names (no `/`, no `..`, no leading `-` or `.`), and `detect.version.cmd` must start with one of them;
  - no control character (escape, newline, tab and the like) anywhere in display text, hints, versions, globs or IDs.
- Install steps use the backends `brew-formula` and `brew-cask`. The `npm` backend is accepted in the file but planned only from Increment 2. `report` and `host-report` steps make an entry `unavailable-here` with a hint.
- The report records the catalog revision as `dir:<absolute path>@<sha256 of the files>`. The built-in catalog is `embedded:<version>`.

A minimal example is [testdata/catalogs/integration/](../testdata/catalogs/integration/), a one-entry catalog with GNU `hello`. The full format is in the [data dictionary, §6](../specs/archive/261114-software-factory-manager/data-dictionary.md#6-catalog-and-profile-yaml-embedded-apiversion-facmanv1) and in the built-in [catalog.yaml](../internal/adapter/catalogyaml/embedded/catalog.yaml).

## Environment variables

`facman` has no environment variables of its own. It reads these standard ones:

| Variable | Used for |
|---|---|
| `HOME` | Finding the state directory and per-user tool locations. Unset or relative is exit 4. |
| `XDG_STATE_HOME` | Linux and WSL: the state directory's parent, when it is an absolute path. |
| `PATH` | Finding `brew` and every detected command. Homebrew is also looked for at `/opt/homebrew` and `/usr/local` (macOS) and `/home/linuxbrew/.linuxbrew` (Linux) when it is not on `PATH`. Only absolute entries are searched: a relative or empty entry (`.`, `bin`, or the empty entry a leading, trailing or doubled `:` makes) is ignored, so a program in the current directory never runs, and `scan` reports a `path-relative-entry` warning. |
| `USER` | The OS user named in the consent record (`user@time`). |
| `DISPLAY`, `WAYLAND_DISPLAY` | Linux: whether GUI apps (casks) are offered. |
| `WSL_DISTRO_NAME` | Detecting WSL, together with `/proc/version`. |
| `HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY` | Downloading the Homebrew installer. Homebrew itself also honors them. |
| `HOMEBREW_*` (yours) | Passed to every `brew` command unchanged, because they are your settings, but they can change what a reviewed install does (`HOMEBREW_CASK_OPTS`, `HOMEBREW_NO_INSTALL_FROM_API`). Review, the `plan`/`apply` header (`brew inherits from your environment: …`), `bootstrap.homebrew.inherited_env` in `--json`, and every operation log (`# inherited HOMEBREW_*: …`) name them, never their values. `HOMEBREW_CASK_OPTS` containing `--no-quarantine` is a `homebrew-cask-opts` warning on `scan`, `plan`, `apply` and `doctor`: unset it for the run (`env -u HOMEBREW_CASK_OPTS facman …`). `HOMEBREW_NO_REQUIRE_TAP_TRUST*` is the exception: it is stripped (see below). |

What `facman` sets for the commands it runs:

- Detection calls get `HOMEBREW_NO_AUTO_UPDATE=1` and `HOMEBREW_NO_ENV_HINTS=1`, so a scan makes no network calls. Install calls do not get them.
- Every child gets `brew shellenv` applied, and `{brew_prefix}/opt/node@24/bin` first on `PATH`.
- `HOMEBREW_NO_REQUIRE_TAP_TRUST*` is removed from every child's environment, so tap trust is always enforced.
- Commands run as an argument list and never through a shell.

**Later.** Mirror overrides and fleet settings (design §9) are not implemented.

## State and logs

| Platform | State directory |
|---|---|
| macOS | `~/Library/Application Support/facman/` |
| Linux, WSL 2 | `${XDG_STATE_HOME:-~/.local/state}/facman/` |

| File | Contents |
|---|---|
| `state.db` | SQLite (WAL). `packages` holds what `facman` manages, with backend, version, path and the consent string. `operations` is the append-only audit log of every action: command, status, exit code, consent and log path. Recent detections are kept too: the last 10 scans. |
| `facman.lock` | Held by `apply` and the TUI for the whole run. A second mutating run exits 4 and names the PID that holds it (or says "PID unknown" when the lock file cannot be read). Read-only commands skip their DB writes while it is held and print a warning. |
| `logs/<YYYYMMDD-HHMMSS>-<operation id>-<package id>.log` | The full output of each command. The header lists the names of the environment variables, never their values. Logs older than 90 days are deleted at launch. `operations` rows are kept forever. Any character of the package ID outside `A-Z a-z 0-9 . _ + @ -` is written as `_`, so a log is always directly in `logs/`. The Homebrew installer's log starts with `# installer: sha256=<hex> bytes=<n>`, the fingerprint of the exact script that ran (Homebrew publishes no checksum); `apply --json` has the same in `results[].installer`. Every log ends with a footer: `# result: exit <n>` (or `killed by signal <NAME> (exit <128+signo>)`, `timed out after <d>; killed`, `cancelled; killed`) and `# time: started …, ended …, took …`. A command that needs your password (the sudo hand-off) writes to your terminal, not to the log; its log holds the header and the footer. |

`facman` never deletes or migrates down a state DB. A DB written by a newer `facman`, or one that is corrupt, is exit 4 with the path. Move the file aside yourself if you want a fresh start.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success, nothing to do, or declined (`n`, empty line, end of input, or `Ctrl+C` at any point before you consent) |
| 1 | An install failed, or a wanted package was skipped (conflict, failed dependency, incomplete detection) or cancelled |
| 2 | `doctor` only: something needs fixing (a conflict, or an untrusted tap a catalog item needs) |
| 3 | Validation: the catalog, a profile, `--profile` or `--with` is wrong |
| 4 | Bootstrap or environment: running as root, a fatal platform finding, missing Linux prerequisites on `apply`, Homebrew prefix owned by another user on `apply`, the lock is held, the state directory is unwritable or unreadable, the installer download failed |
| 64 | Usage: an unknown command or flag, a missing value, a flag or command from a later increment, or `apply`/`facman` without a terminal |

When several apply, the first in this order wins: 64, then 4, then 3, then 1. Usage is checked before any I/O.

## Files from later increments

| File | Flag | Guide |
|---|---|---|
| Configuration list (packages, options and policy for prompt-free runs) | `-cl` (Increment 3) | [configuration-list.md](configuration-list.md) |
| Hermes Agent setup | `-hs` (Increment 5) | [hermes-setup.md](hermes-setup.md) |
| Paperclip setup and company package | `-ps` (Increment 5) | [paperclip-setup.md](paperclip-setup.md) |
| Orca setup | `-os` (Increment 5) | [orca-setup.md](orca-setup.md) |
| T3 Code setup | `-ts` (Increment 5) | [t3-setup.md](t3-setup.md) |
| Templates, schemas and `facman validate` | Increment 4 | [templates-and-validation.md](templates-and-validation.md) |

Back to [user-docs](README.md).
