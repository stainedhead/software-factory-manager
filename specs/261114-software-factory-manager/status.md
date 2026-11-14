# Status: software-factory-manager, Increment 1

**Feature:** software-factory-manager (`facman`), Increment 1 (Phase 0 + Phase 1 MVP)
**Spec directory:** `specs/261114-software-factory-manager/`
**Created:** 2026-11-14
**Orchestration dashboard:** [software-factory-manager-status.md](software-factory-manager-status.md)

> **MANDATORY:** update this file after every task and phase.

## Overall Progress

| Phase | Name | Status | Notes |
|---|---|---|---|
| 0 | Initial research (PRD) | Complete | PRD reviewed 2026-11-14 and moved into this directory |
| 1 | Specification (spec.md) | Complete | Reviewed by `/review-spec` 2026-11-14: implementation-ready |
| 2 | Research & data modeling | Complete | research.md, data-dictionary.md |
| 3 | Architecture & planning | Complete | architecture.md (AD-1 to AD-16), plan.md |
| 4 | Task breakdown | Complete | tasks.md: 52 tasks in 8 workstreams (A to H) |
| 5 | Implementation | In Progress | 21 / 52 tasks. Workstreams A (with T-E1), B and C done; next D, E (parallel), then F, G, H |
| 6 | Completion & archival | Not Started | |

## Phase 0 Checklist

- [x] Spec directory created (`specs/261114-software-factory-manager/`)
- [x] PRD moved into the spec directory (links fixed for the new location)
- [x] Research questions identified (research.md RQ-1 to RQ-8)
- [x] Phase files initialized (spec, research, data-dictionary, architecture, plan, tasks, implementation-notes)
- [x] Spec reviewed (`/review-spec`): Needs revision → Implementation-ready after resolutions R-1 to R-9

## Phase 5 Task Log

| Task | Status | Notes |
|---|---|---|
| T-A1 Makefile targets | ✅ Done | `build` static (`CGO_ENABLED=0`), `build-all` (4 targets), `integration` |
| T-A2 `domain/version` | ✅ Done | 100% coverage |
| T-A3 `domain/platform` | ✅ Done | `Fatal(cmd)` via `Severity` + `BlocksApply` (I-4) |
| T-A4 `domain/failure` | ✅ Done | Exit codes, `Worst` precedence (I-3) |
| T-A5 `domain/catalog` model and `Validate` | ✅ Done | All problems in one validation error |
| T-A6 `domain/catalog` install selection | ✅ Done | `SelectInstall` (I-14) |
| T-A7 `domain/catalog` profile resolution | ✅ Done | `Resolve`, transitive requires (I-7) |
| T-A8 `domain/plan` | ✅ Done | Ordering, invariants, consent (I-6) |
| T-A9 `domain/inventory` | ✅ Done | Transitions, record validation |
| T-A10 `domain/ownership` | ✅ Done | 100% coverage; I-5 |
| T-A11 `fakecmd` and `internal/testkit` | ✅ Done | I-13 |
| T-E1 Ports and fakes | ✅ Done | Ports in `internal/usecase/*/ports.go` (I-8 to I-12) |
| T-B1 YAML decoder | ✅ Done | `file:line:col` errors; `link:` rejected (D-3) |
| T-B2 Embedded catalog v1 | ✅ Done | 30 entries (25 Appendix E + choice members and helpers); I-15 |
| T-B3 Embedded profiles | ✅ Done | `agentic-factory` (includes `base-dev`), `base-dev` |
| T-B4 `--catalog <dir>` source | ✅ Done | Git URL → 64; missing/invalid → 3; `dir:<abs>@<sha256>` |
| T-C1 `adapter/execrunner` | ✅ Done | Argv only, env sanitizing, timeouts, log names-only header, interactive; system clock/user |
| T-C2 `adapter/platform` | ✅ Done | Fixtures + fakecmd for getconf/ldd; 100% coverage |
| T-C3 `adapter/statedir` | ✅ Done | Cross-process flock test with holder PID; I-10 |
| T-C4 `adapter/sqlite` open and migrations | ✅ Done | Newer schema and garbage file → exit 4, file untouched; CGO_ENABLED=0 |
| T-C5 `adapter/sqlite` inventory repository | ✅ Done | Implements scan/execute/status `Inventory`; last 10 scans kept; I-11 |

## Blockers

None. Open questions for J are listed in [spec.md](spec.md#open-questions); each has a default in force, so none blocks Increment 1.

## Recent Activity

| Date (UTC) | Activity |
|---|---|
| 2026-11-14 | `/create-spec` (dev-flow `/implm-frm-prd` Step 1, unattended): spec directory created for Increment 1, PRD moved in, all phase files written. Decisions D-1 to D-17 in implementation-notes.md. |
| 2026-11-14 | `/review-spec` (dev-flow Step 2, unattended): verdict Needs revision (4 Warnings: acceptance criteria, technical approach, component coverage, edge cases), all resolved (R-1 to R-9) → Implementation-ready. Phases 0 to 4 complete. |
| 2026-11-14 | Step 3, workstream A: domain model (version, failure, platform, catalog, plan, inventory, ownership), use case ports (T-E1), fakecmd and testkit, Makefile targets. Deviations I-1 to I-14 in implementation-notes.md. |
| 2026-11-14 | Step 3, workstream B: YAML loader (`adapter/catalogyaml`), embedded catalog v1 and profiles, `--catalog <dir>` source. |
| 2026-11-14 | Step 3, workstream C: `execrunner`, `platform`, `statedir` and `sqlite` adapters. Coverage: 94.7% overall, 100% domain. |
