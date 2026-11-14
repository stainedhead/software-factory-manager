# software-factory-manager

**Agentic Software Factory Manager** — the home of `facman`.

> **Status:** Increment 1 (MVP) is built: the TUI (`facman`) and the headless commands `scan`, `plan`, `apply`, `status` and `doctor`, each with `--json`, on macOS, Linux and WSL 2 through Homebrew. Upgrades, uninstall, adoption, the npm backend, configuration lists (`-cl`), templates and per-tool setup flags arrive in later increments; see [What works today](#what-works-today). The spec is [specs/archive/261114-software-factory-manager/](specs/archive/261114-software-factory-manager/).

## Overview

`software-factory-manager` builds **`facman`**, the **Agentic Software Factory Manager**: a terminal UI and headless CLI that gives lab machines and CI runners a consistent toolset for working in software factories. It detects what is already installed, proposes only what is missing, asks before changing anything, installs through Homebrew (macOS, Linux, WSL) and npm, and then manages only what it installed. Build machines get the same tools with no prompts from a checked-in configuration list.

The repo is named for what it is, a software factory manager. The binary is `facman`, and `facman --version` and `facman --about` both show the full name, "Agentic Software Factory Manager".

Start with [INTENT.md](INTENT.md) for purpose, scope and key decisions, then [docs/product-summary.md](docs/product-summary.md) and the full [design](docs/design/software-factory-installer-design.md).

## Quick start

Requires the latest stable Go (1.27+), and a normal user account (`facman` refuses to run as root, as Homebrew does).

```sh
git clone https://github.com/stainedhead/software-factory-manager.git
cd software-factory-manager
make build                                   # static binary at bin/facman
bin/facman --version
bin/facman scan                              # what is installed, and how (read-only)
bin/facman plan --profile agentic-factory    # the exact commands it would run (read-only)
bin/facman                                   # the TUI: Scan → Plan → Review → Run → Summary
```

Nothing changes on the machine until you consent: press `Enter` on the TUI's Review screen, or answer `y` to `apply`'s `Proceed? [y/N]`. The full walk-through is in [Getting started](user-docs/getting-started.md).

## What works today

| Command | What it does | Changes the machine |
|---|---|---|
| `facman` | The TUI. Opens on a terminal; otherwise prints the about text | Only after `Enter` on Review |
| `facman scan [--json]` | Detects every catalog package, its ownership state, provenance, version and path | No |
| `facman plan --profile P [--with X] [--json]` | Prints the exact, ordered commands that would install what profile `P` is missing | No |
| `facman apply --profile P [--with X] [--dry-run] [--json]` | Shows the plan, asks `Proceed? [y/N]` on the terminal, runs it, records every step | Yes, after `y` |
| `facman status [--json]` | Lists managed, adopted, foreign, drifted and duplicate packages | No |
| `facman doctor [--json]` | Checks the whole catalog for link conflicts and untrusted taps; exit 2 when something needs fixing | No |

Every command except `--version` and `--about` also takes `--catalog <dir>` to use a catalog directory instead of the built-in one. Flags, exit codes and state paths are in [Configuration](user-docs/configuration.md); worked examples are in [Usage examples](user-docs/usage-examples.md).

## Documentation

[INTENT.md](INTENT.md) is the one-page statement of what we are building and why.

**For users and operators** — [user-docs/](user-docs/)

| Guide | What it covers |
|---|---|
| [Getting started](user-docs/getting-started.md) | Build or run `facman`, first scan, plan, apply and the TUI |
| [Usage examples](user-docs/usage-examples.md) | Common tasks: scripting with `--json`, optional packages, custom catalogs, conflicts, diagnosis |
| [Configuration](user-docs/configuration.md) | Every flag, `--catalog`, `--profile`, `--with`, environment variables, state paths and exit codes |
| [Field check](user-docs/field-check.md) | Check detection on real lab Macs (I1-16), run the clean-machine bootstrap, and capture real `brew` output |
| [Configuration lists (`-cl`)](user-docs/configuration-list.md) | *Increment 3:* the same tools on every machine, with no prompts |
| [Templates and validation](user-docs/templates-and-validation.md) | *Increment 4:* templates, schemas and `facman validate` |
| [Hermes setup (`-hs`)](user-docs/hermes-setup.md), [Paperclip setup (`-ps`)](user-docs/paperclip-setup.md), [Orca setup (`-os`)](user-docs/orca-setup.md), [T3 Code setup (`-ts`)](user-docs/t3-setup.md) | *Increment 5:* per-tool setup files |

**For contributors and maintainers** — [docs/](docs/)

| Document | What it covers |
|---|---|
| [Product summary](docs/product-summary.md) | What the product is and why it exists |
| [Product details](docs/product-details.md) | Features, behaviour, and requirements |
| [Technical details](docs/technical-details.md) | Components, data flow, and interfaces |
| [Architecture decision record](docs/architecture-decision-record.md) | Key design decisions and their rationale |
| [Design](docs/design/) | The full working design for `facman` |

Feature specs live in [specs/](specs/); finished ones are in [specs/archive/](specs/archive/). The [Increment 1 (MVP) spec](specs/archive/261114-software-factory-manager/spec.md) is complete and archived; the product PRD is archived with it at [specs/archive/261114-software-factory-manager/software-factory-manager-PRD.md](specs/archive/261114-software-factory-manager/software-factory-manager-PRD.md). The code review's fixes are tracked in their own spec, [specs/261114-software-factory-manager-auto-review/](specs/261114-software-factory-manager-auto-review/).

## Development

| Command | Purpose |
|---|---|
| `make build` | Compile a static (`CGO_ENABLED=0`) `bin/facman` |
| `make build-all` | Cross-compile darwin and linux, amd64 and arm64, into `bin/<os>-<arch>/facman` |
| `make test` | Run tests with the race detector |
| `make integration` | The `TestIntegration*` tests with the `integration` build tag, verbose: read-only checks against real Homebrew (skip without it or as root); `FACMAN_INTEGRATION_MUTATE=1` runs the GNU `hello` install and turns those skips into failures |
| `make lint` | Run golangci-lint |
| `make cover` | Tests with a coverage report (target ≥ 80% overall; Increment 1 is at about 96%) |
| `make fmt` | `gofmt -w .` |
| `make archcheck` | Check the Clean Architecture import rules ([scripts/archcheck](scripts/archcheck/)) |
| `make doccheck` | Check that AGENTS.md lists every make target and later-increment guides carry their status note ([scripts/doccheck](scripts/doccheck/)) |
| `make fuzz` | Fuzz the parsers of external input, 10 s each (`FUZZTIME=…` to change) |
| `make check` | fmt + lint + archcheck + doccheck + test (run before every commit) |

Contributor and AI-agent rules — architecture, TDD, and documentation standards — are in [AGENTS.md](AGENTS.md).

## Repository layout

```
cmd/        entry points (cmd/facman builds the `facman` binary)
internal/   domain, usecase, and adapter layers (Clean Architecture)
docs/       product and technical documentation (docs/design/ holds the full design)
user-docs/  user-facing guides: setup, configuration, adoption
specs/      feature specifications (active and archived)
testdata/   test-only fixtures and the fake command used by tests
scripts/    repo checks run by make (archcheck, doccheck); not shipped
```

Each folder has its own `README.md` describing its purpose and contents.

## License

See [LICENSE](LICENSE).
