# software-factory-manager

**Agentic Software Factory Manager** — the home of `facman`.

> **Status:** design complete in draft (v0.3); Increment 1 (MVP) is specified in [specs/261114-software-factory-manager/](specs/261114-software-factory-manager/) and in implementation: the domain model, use cases, adapters and the headless CLI (`facman scan`, `plan`, `apply`, `status`, `doctor`, each with `--json`) are in place; the TUI is next. Sections marked _TBD_ are filled in as the product takes shape.

## Overview

`software-factory-manager` builds **`facman`**, the **Agentic Software Factory Manager**: a terminal UI and headless CLI that gives lab machines and CI runners a consistent toolset for working in software factories. It detects what is already installed, proposes only what is missing, asks before changing anything, installs through Homebrew (macOS, Linux, WSL) and npm, and then manages only what it installed. Build machines get the same tools with no prompts from a checked-in configuration list.

The repo is named for what it is, a software factory manager. The binary is `facman`, and `facman --version` and `facman --about` both show the full name, "Agentic Software Factory Manager".

Start with [INTENT.md](INTENT.md) for purpose, scope and key decisions, then [docs/product-summary.md](docs/product-summary.md) and the full [design](docs/design/software-factory-installer-design.md).

## Quick start

Requires the latest stable Go (1.27+).

```sh
git clone https://github.com/stainedhead/software-factory-manager.git
cd software-factory-manager
make build   # builds bin/facman
make test
```

## Documentation

[INTENT.md](INTENT.md) is the one-page statement of what we are building and why.

**For users and operators** — [user-docs/](user-docs/)

| Guide | What it covers |
|---|---|
| [Getting started](user-docs/getting-started.md) | Installing and running for the first time |
| [Configuration](user-docs/configuration.md) | Settings, environment variables, and options |
| [Configuration lists (`-cl`)](user-docs/configuration-list.md) | The same tools on every machine, with no prompts |
| [Per-tool setup](user-docs/README.md) | Hermes, Paperclip, Orca and T3 Code setup files, templates and validation |

**For contributors and maintainers** — [docs/](docs/)

| Document | What it covers |
|---|---|
| [Product summary](docs/product-summary.md) | What the product is and why it exists |
| [Product details](docs/product-details.md) | Features, behaviour, and requirements |
| [Technical details](docs/technical-details.md) | Components, data flow, and interfaces |
| [Architecture decision record](docs/architecture-decision-record.md) | Key design decisions and their rationale |
| [Design](docs/design/) | The full working design for `facman` |

Feature specs live in [specs/](specs/); finished ones are in [specs/archive/](specs/archive/). The active spec is [Increment 1 (MVP)](specs/261114-software-factory-manager/spec.md), and the product PRD now lives with it at [specs/261114-software-factory-manager/software-factory-manager-PRD.md](specs/261114-software-factory-manager/software-factory-manager-PRD.md).

## Development

| Command | Purpose |
|---|---|
| `make build` | Compile a static (`CGO_ENABLED=0`) `bin/facman` |
| `make build-all` | Cross-compile darwin and linux, amd64 and arm64, into `bin/<os>-<arch>/facman` |
| `make test` | Run tests with the race detector |
| `make integration` | Run tests including the `integration` build tag (real Homebrew; skips when absent) |
| `make lint` | Run golangci-lint |
| `make cover` | Tests with a coverage report |
| `make check` | fmt + lint + test (run before every commit) |

Contributor and AI-agent rules — architecture, TDD, and documentation standards — are in [AGENTS.md](AGENTS.md).

## Repository layout

```
cmd/        entry points (cmd/facman builds the `facman` binary)
internal/   domain, usecase, and adapter layers (Clean Architecture)
docs/       product and technical documentation (docs/design/ holds the full design)
user-docs/  user-facing guides: setup, configuration, adoption
specs/      feature specifications (active and archived)
testdata/   test-only fixtures and the fake command used by tests
```

Each folder has its own `README.md` describing its purpose and contents.

## License

See [LICENSE](LICENSE).
