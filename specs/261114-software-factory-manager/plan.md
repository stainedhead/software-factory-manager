# Plan: software-factory-manager, Increment 1

**Feature:** software-factory-manager (`facman`)
**Date:** 2026-11-14
**Status:** Planning

## Development Approach

- **TDD everywhere** (AGENTS.md): red → green → refactor; table-driven tests with `t.Run`/`t.Helper`; fakes over mocks; every task in [tasks.md](tasks.md) names its failing test first.
- **Inside-out**: domain rules first (they carry the safety guarantees), then use cases against in-memory port fakes, then adapters against fixtures and `fakecmd`, then the driving adapters (CLI, TUI) and end-to-end tests.
- **Workstreams** land as one or more commits each, every commit green under `make check`, referencing `specs/261114-software-factory-manager/`.
- **Docs in the same change**: each new folder gets its `README.md` in the commit that creates it; parent READMEs updated in the same commit.
- **No Go code beyond the spec without a spec update**: deviations go to [implementation-notes.md](implementation-notes.md#deviations-from-plan) first.

## Phase Breakdown (workstreams)

| WS | Name | Tasks | Depends on | Commit(s) |
|---|---|---|---|---|
| A | Foundation: domain model, failure kinds, `fakecmd`, Makefile | 11 | — | `Add Increment 1 domain model and test harness` |
| B | Catalog v1 data and YAML loader | 4 | A | `Add embedded catalog v1, profiles and YAML loader` |
| C | Infrastructure adapters: runner, platform, state dir, SQLite | 5 | A | `Add runner, platform, state dir and SQLite inventory adapters` |
| D | Detection and Homebrew adapters | 8 | A (C1 for the runner interface shape only) | `Add detectors, Homebrew backends and conflict probe` |
| E | Use cases | 9 | A | `Add bootstrap, scan, planning and execute use cases` |
| F | CLI, report schema, wiring, end-to-end tests | 6 | B, C, D, E | `Add facman CLI commands and end-to-end tests` |
| G | TUI | 5 | E (B for real catalog fixtures in goldens) | `Add the facman TUI install flow` |
| H | Integration tests, CI, docs, quality gate | 4 | F, G | `Add real-Homebrew integration tests and Increment 1 docs` |
| | **Total** | **52** | | |

### Parallelism

```
A ──┬──► B ──┐
    ├──► C ──┼──► F ──┐
    ├──► D ──┤        ├──► H
    └──► E ──┴──► G ──┘
```

- **After A, B, C, D and E run in parallel** (four or five worktrees). They meet only through domain types (from A) and port interfaces (declared in E). D implements ports declared in E: to avoid waiting, E1–E2 (ports) are written first in E, or D codes to the port shapes in [data-dictionary.md §3](data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed) and the integration commit reconciles names.
- **F and G run in parallel** once E is merged (F also needs B, C, D).
- **H is last.**
- **Merge order:** A → (C, B, E, D in any order; each rebased and `go mod tidy`-ed on the integration branch) → F → G → H. `go.mod`/`go.sum` conflicts are resolved by re-running `go mod tidy` after each merge, never by hand.

## Critical Path

A (domain) → E (use cases: scan, planning, execute) → F (CLI + e2e) → H. E is the largest and riskiest workstream (re-plan after bootstrap, crash recovery, dependents skipping); start it first after A and give it the most experienced worker. D's brew JSON parsing is the main external-format risk; T-D2 captures real fixtures early.

## Testing Strategy

| Level | What | Where | Runs in |
|---|---|---|---|
| Domain unit | Classification (all 8 states), profile union and pins, plan ordering and invariants, versions, consent string, exit-code precedence | `internal/domain/**/_test.go` | `make test` (all OSes) |
| Use case unit | Bootstrap decisions, scan with timeouts/`unknown`, reconcile (interrupted, drift, AD-8), planning (installer, tap, trust, exclusions, dependents), execute (op log order, failure skipping, re-plan, stop-after-current, dry run) with in-memory fakes and a fake clock | `internal/usecase/**/_test.go` | `make test` |
| Adapter unit | YAML loader errors with line numbers; brew JSON fixtures; detectors against `fakecmd` and temp dirs; fsprobe with real temp symlinks; SQLite migrations on a temp DB; runner env sanitizing and timeouts; installer download against `httptest` with retries | `internal/adapter/**/_test.go` | `make test` |
| TUI golden | Each screen and key path; fixed width, ASCII profile; `go test ./internal/adapter/tui -update` refreshes | `internal/adapter/tui/testdata/*.golden` | `make test` |
| End-to-end | Build `fakecmd` in `TestMain`; run the app in-process through `cmd/facman` wiring with injected options (and as a built binary for I1-11); temp `HOME`, `XDG_STATE_HOME`, PATH = fake dir only; fixtures under `testdata/fixtures/`; pty for the prompt; invocation log asserted after each test (no `--force`/`--overwrite`, no `upgrade`/`uninstall`, no commands on foreign/duplicate/drifted, no `HOMEBREW_NO_REQUIRE_TAP_TRUST` in env) | `cmd/facman/e2e_test.go` | `make test` (Linux and macOS; no real installs) |
| Integration | Real Homebrew: read-only `scan --json`, `plan`, `doctor` when brew is found and UID ≠ 0; install of `hello` through `--catalog testdata/catalogs/integration` only with `FACMAN_INTEGRATION_MUTATE=1` on darwin | `cmd/facman/integration_test.go` (`//go:build integration`) | CI step `make integration`; macOS sets the env var |

Rules:

- No test touches the developer's real Homebrew, HOME or state dir. End-to-end tests run **in-process**: `cmd/facman/wire.go` builds the app from an unexported `options` struct (Homebrew known prefixes, `/Applications` dirs, `/etc/os-release` and `/proc/version` paths, installer URL, clock, user name, stdio), and `run(args, stdio)` uses `defaultOptions()`. Tests in `package main` pass overrides, plus temp `HOME`, `XDG_STATE_HOME` and a PATH holding only the `fakecmd` links. The binary has **no test-only flags or environment variables**. The crash-recovery test (I1-11) builds the real binary and runs it as a subprocess with the fake `brew` on PATH (so known prefixes are never consulted) and a small `--catalog`. See implementation-notes D-11.
- On Linux CI, integration tests find Homebrew at `/home/linuxbrew/.linuxbrew` and run read-only checks only; without the env var nothing is installed. Without Homebrew they skip with a message.
- Coverage: `make cover` ≥ 80% overall; domain and usecase ≥ 90%.
- Timing tests use generous bounds (scan < 5 s on fixtures) to avoid flakes; reference-hardware timings are measured manually and recorded.

## Rollout Strategy

- Lands on branch `claude/project-thread-ys3kmd`, PR to `main` at dev-flow Step 14; J merges.
- No release tag in Increment 1 is required; when J tags `v0.1.0`, GoReleaser publishes unsigned binaries with checksums for lab testing (OQ-13 open for signing).
- After merge, J runs `facman scan --json` on 3 to 5 lab Macs (I1-16) and feeds corrections into the catalog (data-only changes).
- Next: Increment 2 or 3 (J may swap them) gets a new spec dir.

## Success Metrics

| Metric | Target |
|---|---|
| I1-1 to I1-15 | All green in CI on Linux and macOS |
| Coverage | ≥ 80% overall; ≥ 90% domain and usecase |
| Safety invariants | Zero recorded invocations with `--force`/`--overwrite`/`upgrade`/`uninstall` across the suite |
| Performance (fixtures) | `scan` < 5 s; `--version` well under 100 ms on CI |
| Field check (I1-16) | Provenance correct for ≥ 90% of detected tools on J's lab Macs; every miss becomes a catalog fix |
