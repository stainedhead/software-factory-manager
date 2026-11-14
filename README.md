# software-factory-manager

> **Status:** design complete in draft (v0.3), implementation not started. Sections marked _TBD_ are filled in as the product takes shape.

## Overview

`software-factory-manager` builds **`sfi`**, a terminal UI and headless CLI that gives lab machines and CI runners a consistent toolset for working in software factories. It detects what is already installed, proposes only what is missing, asks before changing anything, installs through Homebrew (macOS, Linux, WSL) and npm, and then manages only what it installed. Build machines get the same tools with no prompts from a checked-in configuration list.

Start with [INTENT.md](INTENT.md) for purpose, scope and key decisions, then [docs/product-summary.md](docs/product-summary.md) and the full [design](docs/design/software-factory-installer-design.md).

## Quick start

Requires Go 1.22+.

```sh
git clone https://github.com/stainedhead/software-factory-manager.git
cd software-factory-manager
make build   # binaries land in bin/
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
| [Design](docs/design/) | The full working design for `sfi` |

Feature specs live in [specs/](specs/); finished ones are in [specs/archive/](specs/archive/).

## Development

| Command | Purpose |
|---|---|
| `make build` | Compile binaries into `bin/` |
| `make test` | Run tests with the race detector |
| `make lint` | Run golangci-lint |
| `make cover` | Tests with a coverage report |
| `make check` | fmt + lint + test (run before every commit) |

Contributor and AI-agent rules — architecture, TDD, and documentation standards — are in [AGENTS.md](AGENTS.md).

## Repository layout

```
cmd/        entry points
internal/   domain, usecase, and adapter layers (Clean Architecture)
docs/       product and technical documentation (docs/design/ holds the full design)
user-docs/  user-facing guides: setup, configuration, adoption
specs/      feature specifications (active and archived)
```

Each folder has its own `README.md` describing its purpose and contents.

## License

See [LICENSE](LICENSE).
