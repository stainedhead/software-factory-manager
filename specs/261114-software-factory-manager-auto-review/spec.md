# Spec: software-factory-manager auto-review fixes (Increment 1)

**Created:** 2026-11-14 (dev-flow `/implm-frm-prd` Step 8, `/create-spec`, run unattended)
**Source PRD:** [software-factory-manager-auto-review-PRD.md](software-factory-manager-auto-review-PRD.md) (moved here from the repo root)
**Spec it fixes:** [../archive/261114-software-factory-manager/spec.md](../archive/261114-software-factory-manager/spec.md)
**Status:** Implementation

## Executive Summary

The Step 5 code review of `facman` Increment 1 found 19 issues (0 P0, 3 P1, 16 P2), split by the Step 6 PRD review into 26 atomic requirements. The P1s are security and evidence gaps. Values from a `--catalog` directory reach argv, log paths and the consent screen unvalidated, and the `--force` guard misses `-f`. Relative `PATH` entries are searched for programs. The clean-machine bootstrap has never run against real Homebrew. The P2s cover robustness, observability, layering and documentation. This spec fixes them in priority order with TDD, and defers, with a written rationale, the items that cannot be done safely or usefully in this run.

## Problem Statement

Increment 1 passes its acceptance criteria in CI, but the Security NFR ("no `--force`/`--overwrite`", per-item tap trust, consent shows the exact commands) holds only for the embedded catalog. Catalogs are meant to be shared between factories, so they are input and must be validated. See the PRD's Problem Statement and the per-FR findings.

## Goals / Non-Goals

**Goals** (PRD RG1 to RG4):

- **RG1.** Every Security NFR invariant holds for any catalog `facman` accepts (FR-001, FR-002, FR-011, FR-012, FR-013).
- **RG2.** The clean-machine bootstrap is proven, or has an explicit field procedure, and I1-14 evidence is auditable (FR-003, FR-004, FR-019).
- **RG3.** A failed or hung run leaves enough in the state directory to say why (FR-005 to FR-009).
- **RG4.** Layer boundaries and docs match AGENTS.md (FR-010, FR-014 to FR-018).

**Non-Goals:** no new commands, backends or Increment 2 scope; no change to the consent model, exit codes (Appendix H), report schema v1 field names or the state DB schema; no TUI rendering rewrite; no installer checksum.

## User Requirements

### Functional Requirements

Each requirement and its acceptance criteria are stated in full in the PRD. Summary, in implementation order:

| ID | Pri | Requirement | Task |
|---|---|---|---|
| FR-001a | P1 | `catalog.Validate` rejects package IDs, brew install IDs, taps and `detect.commands` outside strict allow-lists, and any control character | T-1a |
| FR-001b | P1 | `plan.Validate` rejects every option in brew argv after the subcommand except `--cask`/`--formula` on `install`/`trust` | T-1b |
| FR-001c | P1 | `Command.String` escapes every control character; Review and headless plan render through it | T-1c |
| FR-002 | P1 | Program lookup and the PATH detector skip non-absolute PATH entries; `scan` warns `path-relative-entry` | T-2 |
| FR-003 | P1 | Clean-machine bootstrap and brew parsers proven against real Homebrew | T-3 |
| FR-001d | P2 | Log file names keep the package ID inside `logs/` | T-1d |
| FR-004 | P2 | `make integration` is verbose; `FACMAN_INTEGRATION_MUTATE=1` turns skips into failures | T-4 |
| FR-005a | P2 | Atomic schema migration under `BEGIN IMMEDIATE` | T-5a |
| FR-005b | P2 | Lock-held message never says "PID 0" | T-5b |
| FR-006 | P2 | A second interrupt stops the run; truthful first-interrupt text | T-6 |
| FR-007a | P2 | Every operation log ends with an outcome footer | T-7a |
| FR-007b | P2 | Signal-killed commands record `128+signo` and `killed by signal <NAME>` | T-7b |
| FR-008 | P2 | Read-only probes logged; every `unknown` detection has a reason | T-8 |
| FR-009 | P2 | Installer: over-limit body fails, no non-https redirects, SHA-256 and size recorded | T-9 |
| FR-010a | P2 | Pre-flight and re-plan rules move to the use-case layer | T-10a |
| FR-010b | P2 | `adapter/tui` stops importing `adapter/cli` | T-10b |
| FR-010c | P2 | Architecture rules documented and enforced by `make archcheck` | T-10c |
| FR-011 | P2 | A plan that fails `plan.Validate` is never printed | T-11 |
| FR-012 | P2 | Plan commands run the located brew by absolute path | T-12 |
| FR-013 | P2 | Inherited `HOMEBREW_*` names shown; `HOMEBREW_CASK_OPTS=--no-quarantine` warning | T-13 |
| FR-014 | P2 | Fuzz targets for external-input parsers and `make fuzz` | T-14 |
| FR-015 | P2 | AGENTS.md Commands table lists every Makefile target, checked by a script | T-15 |
| FR-016 | P2 | Paperclip's unavailable reason printed once | T-16 |
| FR-017 | P2 | Acceptance evidence table, field-check procedure, catalog/DB format docs outside the archive | T-17 |
| FR-018 | P2 | Future-increment user guides carry a status note, checked by a script | T-18 |
| FR-019 | P2 | CI runs `make build-all`; third-party actions pinned by SHA or ADR rationale | T-19 |

## Non-Functional Requirements

As the PRD's NFR table: Security (no option or control character from data reaches brew argv or Review; no program from a relative PATH entry), Reliability (no fix changes a consented argv), Performance (spec bounds still hold), Observability, Compatibility (report schema v1 gains optional fields only; DB schema unchanged; exit codes unchanged), Maintainability (`make check` green, coverage ≥ 80% overall, ≥ 90% on domain and usecase, new code covered).

## System Architecture

Affected layers:

- **domain:** `catalog` (value allow-lists, FR-001a), `plan` (argv option invariant FR-001b, display escaping FR-001c).
- **usecase:** `scan` (relative PATH finding FR-002), `planning` (FR-011, FR-016).
- **adapter:** `execrunner` (FR-002, FR-007), `detect` (FR-002), `statedir` (FR-001d, FR-005b), `sqlite` (FR-005a), `installer` (FR-009), `cli`/`tui` (FR-010b, FR-011, FR-013).
- **tooling/docs:** `Makefile`, `scripts/` (FR-010c, FR-015, FR-018), `.github/workflows` (FR-004, FR-019), `docs/`, `user-docs/`, AGENTS.md.

No new dependency.

## Scope of Changes

See [plan.md](plan.md) for files per task. All changes are internal hardening; no flag is added or removed.

## Breaking Changes

- **Catalog format:** catalogs with IDs, taps or commands outside the allow-lists now fail to load with exit 3. The embedded catalog passes unchanged. This is intended (FR-001a, OQ-4).
- **API, config, schema:** none. Report schema v1 gains only the finding code `path-relative-entry` (codes are an open set).

## Success Criteria and Acceptance Criteria

- Every implemented FR's acceptance criteria pass (`make check`, CI on Linux and macOS).
- Each fix's commit includes its regression test, and its review is recorded in [implementation-notes.md](implementation-notes.md).
- The FR-001 reproduction (`id: "-f"` in a `--catalog`) exits 3 with no brew invocation.
- Every deferred FR has a rationale in implementation-notes.md and an open item in status.md.
- Quality gates: `PATH=$HOME/go/bin:$PATH make check` before every commit; `make cover` ≥ 80% overall.

## Risks and Mitigation

| Risk | Mitigation |
|---|---|
| Allow-lists reject a real cask or tap (OQ-4) | Patterns from the PRD; widen only with a catalog entry that needs it and a test |
| Running real Homebrew here would mutate the machine | Never run; FR-003 becomes a field-test checklist for J |
| Behaviour regressions from refactors (FR-010a) | Gate on the unchanged e2e suite; defer if the refactor is not justified in this run |

## Timeline and Milestones

Single unattended run on 2026-11-14 (dev-flow Step 9): P1 fixes first, then P2s.

## References

- Source PRD: [specs/261114-software-factory-manager-auto-review/software-factory-manager-auto-review-PRD.md](software-factory-manager-auto-review-PRD.md)
- Increment 1 spec: [../archive/261114-software-factory-manager/spec.md](../archive/261114-software-factory-manager/spec.md)
- [AGENTS.md](../../AGENTS.md), [INTENT.md](../../INTENT.md)
