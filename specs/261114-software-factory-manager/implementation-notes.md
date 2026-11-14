# Implementation Notes: software-factory-manager, Increment 1

**Feature:** software-factory-manager (`facman`)
**Date:** 2026-11-14

## Purpose

The running record of decisions, edge cases, deviations and lessons for Increment 1. Add an entry whenever a choice is made that the spec does not already state, a spec item turns out wrong, or something surprising is learned. Each entry: ID, date, decision, why, and where it is reflected. Decisions made while J was away are defaults J can overturn.

## Technical Decisions

Decisions made while creating the spec (`/create-spec`, 2026-11-14, unattended). Rules applied, in order: PRD (Delivery plan first), INTENT.md, AGENTS.md, design doc, then safest default and consent first.

| ID | Decision | Why | Reflected in |
|---|---|---|---|
| D-1 | The spec covers **Increment 1 only** (Phase 0 + Phase 1), using the PRD's FR numbers with Full/Partial scope, and lists every other FR as deferred. | The PRD Delivery plan says each increment gets its own spec; one spec for the whole PRD would not be implementable as one change. | spec.md |
| D-2 | CLI and TUI are driving adapters (`internal/adapter/cli`, `internal/adapter/tui`); `cmd/facman` only wires. | Keeps `cmd` thin (AGENTS.md) while making exit codes, the prompt and screens testable. | architecture.md AD-2 |
| D-3 | No `brew` invocation ever contains `--force`, including `brew link --force node@24`. The `node@24` keg `bin` goes first on children's PATH, and Summary prints the profile line. | I1-9 requires that no recorded invocation contains `--force`; the design offered the PATH alternative; FR-053's profile write is deferred anyway. | spec.md (Edge cases, FR-053), architecture.md AD-10, data-dictionary §2.3 |
| D-4 | `facman` with no args opens the TUI only when stdout is a terminal; otherwise prints `--about` and a hint, exit 0. Unknown flags now exit 64 (was 2). | Avoids hanging pipes and CI smoke steps; aligns with Appendix H. | spec.md (Breaking Changes), architecture.md AD-14 |
| D-5 | Decline includes `n`, empty line, EOF and `Ctrl+C` at the prompt. All exit 0 with no rows. | Default No; nothing happened. | spec.md |
| D-6 | Exit precedence: usage (64) → bootstrap (4) → validation (3) → operation (1) → 0; `doctor` alone returns 2. | Deterministic exit codes (NFR); usage is checked before any I/O. | spec.md (Edge cases), data-dictionary §2.11 |
| D-7 | A package whose detection is incomplete (a relevant detector timed out or failed) is never planned and counts toward exit 1 on `apply`. | Safest default: do not install on top of something we could not see. | spec.md (Edge cases) |
| D-8 | Interrupted install reconciliation: if the package is now detected through the operation's backend, record it `managed`; otherwise leave it `absent` so the next `apply` proposes it. | The install was consented; ownership stays honest (I1-11). | architecture.md AD-8 |
| D-9 | Orca's conflict pre-flight before its tap exists uses the catalog's `detect.commands` under `{brew_prefix}/bin` and its app bundle name under `/Applications`, because `brew info --cask stablyai/orca/orca` fails until the tap is added. | Pre-flight must not mutate (no tap during planning). | research.md RQ-1, tasks T-E4 |
| D-10 | Mutating runs take an exclusive `flock` on `<state dir>/facman.lock`; a second one exits 4 naming the holder PID. Read-only commands skip DB writes when locked. | Prevents double installs and log corruption. | architecture.md AD-9 |
| D-11 | End-to-end tests run in-process through `cmd/facman` wiring with injected options; the binary has no test-only flags or environment variables. Only the crash test runs the built binary (fake `brew` on PATH, small `--catalog`). | CI runners have real Homebrew at known prefixes; tests must never see it, and a hidden test knob in a shipped binary is a needless risk. | plan.md (Testing Strategy), tasks T-F4, T-F5 |
| D-12 | I1-14 installs GNU `hello` through a one-entry `--catalog testdata/catalogs/integration` (profile `integration`), only when `FACMAN_INTEGRATION_MUTATE=1`; CI sets it on macOS only. Read-only integration checks run wherever Homebrew is found and UID ≠ 0; otherwise the tests skip. | CI runs `-tags integration` on Linux and macOS; mutation must be opt-in. | spec.md I1-14, tasks T-H1, T-H2 |
| D-13 | `--with <member-id>` picks a choice-group member headlessly (e.g. `--with colima`); `--with container-runtime` alone is exit 3 listing the members. | `--set` is deferred, and the group must name its pick (FR-042). | spec.md FR-079, tasks T-A7 |
| D-14 | Catalog entries gain `sign_in_required: true` for the five entitlement tools. | FR-112 needs the list as data, not code. | data-dictionary §6, tasks T-B2 |
| D-15 | Detection calls set `HOMEBREW_NO_AUTO_UPDATE=1` and `HOMEBREW_NO_ENV_HINTS=1`; install calls do not. | Detection makes no network calls (NFR); installs behave as brew normally does. | research.md RQ-1, architecture.md AD-13 |
| D-16 | UID 0 is refused (exit 4) for every command except `--version`, `--about`, `--help`. | Homebrew refuses root, and a root scan would write state into root's home. | spec.md FR-056, Edge cases |
| D-17 | YAML library `go.yaml.in/yaml/v3` (the maintained successor of `gopkg.in/yaml.v3`). | Line/column info through the node API; maintained. | spec.md Dependencies |

## Edge Cases & Solutions

See [spec.md, Edge Cases and Error Handling](spec.md#edge-cases-and-error-handling) for the agreed behaviour. Add entries here as implementation finds new ones.

| Case | Solution | Test |
|---|---|---|
| — | — | — |

## Deviations from Plan

None yet. Record any change from spec.md, architecture.md or tasks.md here **before** making it, then update the affected spec file.

## Measurements

Fill in during T-F6 and the I1-16 field run.

| Metric | Target | Fixture/CI | Reference Mac | ubuntu-24.04 |
|---|---|---|---|---|
| `--version` | < 100 ms | | | |
| First TUI frame | < 500 ms | | | |
| Bootstrap check (brew present) | < 2 s | | | |
| `scan` full catalog (p95) | < 5 s | | | |

## Lessons Learned

To be written at the end of Increment 1.
