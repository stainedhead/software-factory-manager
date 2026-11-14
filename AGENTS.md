# AGENTS.md

Rules for AI agents and human contributors working in `software-factory-manager` (Go). The repo builds one application, **`facman`** — the **Agentic Software Factory Manager** (entry point `cmd/facman`). Always use that full name in user-visible text; it comes from `internal/domain/appinfo`, so never hard-code it elsewhere. The design's old working name `sfi` is retired — never use it.

Before changing code, read `INTENT.md` (purpose, scope, key decisions), the root `README.md`, the `README.md` in the folders you touch, and `docs/technical-details.md`. The full working design is in `docs/design/`; the per-tool guides are in `user-docs/`.

## Architecture — Clean Architecture

Dependencies point inward only:

```
cmd/facman/      entry point for the `facman` binary (thin: wiring and flags only)
internal/
  domain/       entities, value objects, domain errors — no external imports
  usecase/      application logic; defines the interfaces it needs (ports)
  adapter/      implementations of ports (HTTP, DB, filesystem, CLIs)
```

- `domain` imports nothing from this repo's other layers.
- `usecase` imports `domain` only. Interfaces are declared where they are consumed.
- `adapter` and `cmd` may import inward layers; never the reverse.
- Wire dependencies explicitly in `cmd/`; no globals or `init()` side effects.
- **Exception: `internal/usecase/process`.** It holds only the value types of the command-runner port (`Cmd`, `Result`, `ErrTimeout`, `ErrNotFound`) and no interfaces. Every use case that runs commands imports it and declares its own `CommandRunner` interface over those types, so one adapter (`execrunner`) satisfies them all. Keep it value types only.
- The two front ends, `adapter/cli` and `adapter/tui`, never import each other; `cmd/facman` adapts one to the other (`cli.TUIFunc`).
- `make archcheck` (part of `make check` and CI) enforces these rules: `domain` imports only `domain` and the standard library, `usecase` never imports `adapter`, `cmd` or `testkit`, and `cli`/`tui` never import each other.

## Commands

Use the `Makefile`; do not invent ad-hoc invocations.

| Command | Purpose |
|---|---|
| `make build` | Compile binaries from `./cmd/...` into `bin/` (produces `bin/facman`) |
| `make build-all` | Cross-compile the release matrix (darwin/linux × amd64/arm64) into `bin/<os>-<arch>/facman`; CI runs it on Linux |
| `make test` | `go test -race ./...` |
| `make integration` | The `TestIntegration*` tests with `-tags integration`, verbose: read-only checks against real Homebrew, which skip without Homebrew or as root. `FACMAN_INTEGRATION_MUTATE=1` (CI sets it on macOS only) also installs and removes GNU `hello`, and turns those skips into failures. Never set it on a machine you care about |
| `make lint` | `golangci-lint run` (v2, config in `.golangci.yml`) |
| `make fmt` | `gofmt -w .` |
| `make cover` | Tests with coverage report; target **≥ 80%** overall, new code fully covered |
| `make archcheck` | Check the Clean Architecture import rules above (`scripts/archcheck`) |
| `make doccheck` | Check that every `.PHONY` target has a row in this table and every later-increment user guide carries its status note (`scripts/doccheck`) |
| `make fuzz` | Fuzz each external-input parser for `FUZZTIME` (default 10 s); CI runs it on Linux. Commit a crasher's `testdata/fuzz/<FuzzName>/` file with the fix |
| `make check` | fmt + lint + archcheck + doccheck + test — must pass before every commit |

## Development — TDD

1. Write a failing test first, then the minimal code to pass, then refactor (red-green-refactor).
2. Table-driven tests; use `t.Run` and `t.Helper`. Prefer fakes over mocks.
3. Every bug fix starts with a regression test.
4. Unit tests live beside the code (`*_test.go`); slower integration tests go behind the `integration` build tag and run in CI.
5. Run `make check` before any commit.

## Go toolchain and conventions

- Use the latest stable Go (currently 1.27); the `go` directive in `go.mod` is the single source, and CI and releases read it. Bump it when a new Go release ships (module `github.com/stainedhead/software-factory-manager`).
- Run `go mod tidy` whenever imports change and commit `go.mod`/`go.sum` with the code.
- Format with `gofmt`/`goimports`; lint with `golangci-lint` (`.golangci.yml`). Fix findings, don't suppress them without a comment explaining why.
- Wrap errors with context: `fmt.Errorf("doing x: %w", err)`. No panics in library code.
- Pass `context.Context` as the first parameter of anything that does I/O.
- Keep packages small and named for what they provide; avoid `util`/`common`.
- Exported identifiers need doc comments.

## Documentation — keep it current

Documentation is part of the change. A change is not done until the affected docs are updated in the same commit/PR.

| File | Audience | Purpose | Update when |
|---|---|---|---|
| `README.md` (root) | Everyone | Overview, quick start, links to all docs | Features, commands, or setup change |
| `docs/product-summary.md` | Product/stakeholders | One-page what and why | Scope or positioning changes |
| `docs/product-details.md` | Product/engineering | Features, behaviour, requirements | A feature is added, changed, or removed |
| `docs/technical-details.md` | Engineers | Components, data flow, interfaces, dependencies | Structure, interfaces, or dependencies change |
| `INTENT.md` (root) | Everyone | One-page purpose, scope, and key decisions | Purpose, scope, or a key decision changes |
| `docs/design/` | Engineers | Full working design for `facman` | The design changes; keep in step with the code |
| `docs/architecture-decision-record.md` | Engineers | Dated log of architectural decisions and rationale | Any significant design decision is made or reversed |
| `user-docs/` | Users/operators | Getting started, configuration, adoption guides | User-visible behaviour or configuration changes |
| `specs/` | Engineers | Active feature specs; finished ones go to `specs/archive/` | Per the specification workflow below |

## README.md at every level

- Keep a `README.md` at the repo root **and in every directory of the package hierarchy** (`cmd/`, `cmd/facman/`, `internal/<layer>/`, `internal/<layer>/<package>/`, `docs/`, `docs/design/`, `user-docs/`, `specs/`).
- Each folder README states: the folder's purpose, its key files/subpackages, how it fits the architecture (what it depends on and what depends on it), and links to related docs.
- When you add, remove, move, or change the responsibility of a folder or package, update its README and its parent's README in the same change. Create the README when you create the folder.
- **Read the READMEs first** when learning or navigating the codebase: start at the root `README.md`, then the README of each folder you work in. Treat them, together with `docs/`, as the source of truth for intent; if code and docs disagree, fix whichever is wrong.

## CI/CD

Every push to `main` and every pull request runs the full pipeline; a release runs on a version tag.

| Trigger | Workflow | What it does |
|---|---|---|
| Pull request, push to `main` | `.github/workflows/ci.yml` | `go mod tidy` check, `golangci-lint`, `make archcheck`, `make doccheck`, `make cover`, `make integration`, `make build` and a `facman --version` smoke test, on Linux and macOS; on Linux also `make fuzz` and `make build-all` |
| Tag `v*` | `.github/workflows/release.yml` | `make check`, then GoReleaser builds linux/darwin (amd64, arm64) binaries and publishes them with checksums to GitHub Releases (`.goreleaser.yaml`) |

Rules:
- **CI must be green.** Direct pushes to `main` are allowed, but a red `main` is fixed before any other work.
- **CI mirrors `make check`.** Anything CI enforces must also be runnable locally through the `Makefile`; when you add a CI step, add or extend a make target.
- **Releases are tags.** Cut a release with an annotated semver tag (`git tag -a v0.1.0 -m ...` then push it). Never build or publish release artifacts by hand. The version is injected into `facman --version` at build time.
- **Contract tests gate releases.** Once the catalog and format contracts exist, no release ships unless they pass against the pinned tool versions (see `docs/design/`). A nightly job against latest tool releases will be added then.
- **Not yet in place:** binary signing and notarization, the Homebrew tap, and the nightly job. Add them as the design calls for them and record the decision in `docs/architecture-decision-record.md`.
- Keep workflow files and `.goreleaser.yaml` documented in `.github/README.md`, and update it when they change.

## Git workflow

- Direct pushes to `main` are allowed; a pull request is optional. Run `make check` before pushing, and watch CI after (see CI/CD above).
- Small, focused commits with imperative-mood messages.

## Dev-Flow Skills

The following skills are available for the developer workflow. Invoke them with the listed slash command.

| Skill | Invocation | Purpose |
|---|---|---|
| `init-dev-flow` | `/init-dev-flow` | One-time repo setup; updates AGENTS.md |
| `create-prd` | `/create-prd [title]` | Interactive PRD authoring; writes to repo root |
| `review-prd` | `/review-prd [file]` | PRD quality review |
| `create-spec` | `/create-spec [prd-file]` | Spec creation from PRD; YYMMDD-prefixed directory |
| `review-spec` | `/review-spec [spec-dir]` | Spec quality review |
| `archive-spec` | `/archive-spec [spec-dir]` | Move completed spec to specs/archive/ |
| `implm-frm-prd` | `/implm-frm-prd [prd-file]` | Implement from PRD (11 steps) |
| `implm-frm-change-dtls` | `/implm-frm-change-dtls [ticket-or-desc]` | Implement from ticket or description (12 steps) |
| `implm-from-spec` | `/implm-from-spec [spec-dir]` | Full 11-step orchestrated implementation |
| `create-review` | `/create-review` | Code and design review artifact |
| `review-code` | `/review-code` | Code review of current branch |
| `write-flow-analys` | `/write-flow-analys [spec-dir]` | Process analysis report (final step of implm-from-spec) |

## Feature Specification Workflow

### Specs Directory Structure

All feature development uses the `specs/` directory for planning and tracking. Each feature gets its own subdirectory named `YYMMDD-<feature-name>/`.

**Directory Structure:**
```
specs/
└── YYMMDD-<feature-name>/
    ├── spec.md                  # Feature specification and requirements
    ├── status.md                # **CRITICAL**: Phase progress tracking (update after each task)
    ├── plan.md                  # Implementation plan and architecture decisions
    ├── tasks.md                 # Task breakdown and progress tracking
    ├── research.md              # Research findings, API docs, examples
    ├── data-dictionary.md       # Data structures, types, schemas
    ├── architecture.md          # System architecture and component design
    └── implementation-notes.md  # Implementation details, gotchas, decisions
```

### Progressive Documentation Build

Documents are created progressively as the feature develops:

**Phase 0: Initial Research (PRD/Feature Research)**
- Input: Product Requirement Document, RFC, or feature research
- Purpose: Understand the problem, gather requirements, identify constraints
- **Update status.md**: Mark Phase 0 as "In Progress"

**Phase 1: Specification (spec.md)**
- Define what the feature does, user requirements, acceptance criteria, goals and non-goals
- **Update status.md**: Mark Phase 0 complete, Phase 1 in progress

**Phase 2: Research & Data Modeling (research.md, data-dictionary.md)**
- Gather documentation, explore existing code, define domain entities and data structures
- **Update status.md**: Mark Phase 1 complete, Phase 2 in progress

**Phase 3: Architecture & Planning (architecture.md, plan.md)**
- Design implementation approach, identify affected layers, document component architecture
- **Update status.md**: Mark Phase 2 complete, Phase 3 in progress

**Phase 4: Task Breakdown (tasks.md)**
- Break down work into concrete, testable tasks with dependencies and estimates
- **Update status.md**: Mark Phase 3 complete, Phase 4 in progress

**Phase 5: Implementation (code + implementation-notes.md)**
- Follow TDD (Red-Green-Refactor), record decisions, document edge cases
- **Update status.md**: After EACH task completion — MANDATORY

**Phase 6: Completion & Archival**
- Update product documentation, move spec to `specs/archive/`, capture lessons learned
- **Verify status.md**: Must show 100% completion before archiving

**MANDATORY**: Update `status.md` after completing each task or phase. This file is the single source of truth for progress tracking.

### Specs Workflow Rules

- **Create the spec directory** before starting any new feature work
- **Update progressively** — specs are living documents, not written once
- **Update status.md ALWAYS** after completing each task, phase, or milestone — this is MANDATORY
- **Reference from commits** — link to the spec directory in commit messages
- **Archive completed specs** — move to `specs/archive/` when 100% complete in status.md
- **Version control** — commit specs alongside code for team visibility
