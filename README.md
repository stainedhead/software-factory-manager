# software-factory-manager

> **Status:** early scaffolding. This README describes the project layout and workflow; sections marked _TBD_ are filled in as the product takes shape.

## Overview

_TBD — one paragraph on what software-factory-manager does and who it is for. See [docs/product-summary.md](docs/product-summary.md)._

## Quick start

Requires Go 1.22+.

```sh
git clone https://github.com/stainedhead/software-factory-manager.git
cd software-factory-manager
make build   # binaries land in bin/
make test
```

## Documentation

**For users and operators** — [user-docs/](user-docs/)

| Guide | What it covers |
|---|---|
| [Getting started](user-docs/getting-started.md) | Installing and running for the first time |
| [Configuration](user-docs/configuration.md) | Settings, environment variables, and options |

**For contributors and maintainers** — [docs/](docs/)

| Document | What it covers |
|---|---|
| [Product summary](docs/product-summary.md) | What the product is and why it exists |
| [Product details](docs/product-details.md) | Features, behaviour, and requirements |
| [Technical details](docs/technical-details.md) | Components, data flow, and interfaces |
| [Architecture decision record](docs/architecture-decision-record.md) | Key design decisions and their rationale |

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
docs/       product and technical documentation
user-docs/  user-facing guides: setup, configuration, adoption
specs/      feature specifications (active and archived)
```

Each folder has its own `README.md` describing its purpose and contents.

## License

See [LICENSE](LICENSE).
