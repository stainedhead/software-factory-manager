# Architecture: software-factory-manager auto-review fixes

**Date:** 2026-11-14
**Status:** Draft → Accepted for implementation

## Architecture Overview

No new components. The fixes harden the existing Clean Architecture layers (see [../archive/261114-software-factory-manager/architecture.md](../architecture.md)).

## Component Architecture

| Fix | Layer | Package |
|---|---|---|
| Value allow-lists | domain | `catalog` (`Validate`) |
| Option invariant, display escaping | domain | `plan` (`Validate`, `Command.String`) |
| Relative PATH | adapter + usecase | `execrunner`, `detect`; finding in `usecase/scan` |
| Log name sanitizing, lock PID | adapter | `statedir` |
| Installer hardening | adapter | `installer` |

## Layer Responsibilities

- **domain** owns the rules that make a catalog acceptable and a plan safe. Both are pure functions over values.
- **adapters** never trust their inputs to be safe beyond what the domain guarantees, so path- and terminal-facing adapters keep their own defence (log names).

## Data Flow

Catalog YAML → `catalogyaml` decode → `catalog.Validate` (FR-001a, exit 3 on failure) → planning → `plan.Validate` (FR-001b, exit 1 as an internal error) → Review via `Command.String` (FR-001c) → execute.

## Sequence Diagrams

Unchanged from Increment 1.

## Integration Points

Homebrew CLI (argv), the Homebrew installer (HTTPS), the shell `PATH`.

## Architectural Decisions

- **AR-1.** Validate at load *and* keep the domain invariant: two independent lines of defence, as the PRD asks.
- **AR-2.** Escape control characters in display with Go quoting (`strconv.Quote`), the same quoting already used for metacharacters.
