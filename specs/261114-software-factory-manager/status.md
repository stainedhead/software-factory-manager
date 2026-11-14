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
| 5 | Implementation | In Progress | 38 / 52 tasks (+ T-D9). Workstreams A (with T-E1), B, C, D and E done; F in progress; then G, H |
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
| T-D1 Locate brew and `shellenv` | ✅ Done | `adapter/brew` `Locator`; shellenv parsed without a shell |
| T-D2 Parse `brew info --json=v2` | ✅ Done | Fixtures shaped from research.md, **not captured from real Homebrew**: confirm in I1-16/I1-14 (I-18) |
| T-D3 Brew detector | ✅ Done | One `info --installed` call, read-only env, 10 s |
| T-D4 `brew-formula`/`brew-cask` backends | ✅ Done | Tap/trust as separate actions; sudo default when artifacts unreadable (I-19, I-20) |
| T-D5 PATH detector | ✅ Done | `/usr/bin/git` never run without CLT (fakecmd log); I-22 |
| T-D6 App-bundle detector | ✅ Done | mdfind, fallback scan via plutil |
| T-D7 npm detector | ✅ Done | |
| T-D8 `adapter/fsprobe` | ✅ Done | Evidence and owner classes; I-23 |
| T-D9 Detection-only `gemini-cli` | ✅ Done | `superseded_by`; scan finding lands with T-E3 (I-16) |
| T-E2 `usecase/bootstrap` | ✅ Done | `Check`, `ChildEnv`; 100% coverage |
| T-E3 `usecase/scan` | ✅ Done | Parallel detectors, `Covers`, AD-8 reconcile, superseded finding; I-26 |
| T-E4 `usecase/conflicts` | ✅ Done | `Check`, `CheckAll` (unreadable → `Failed`); 100% coverage |
| T-E5 `usecase/planning` | ✅ Done | Installer first, tap/trust chain, exclusions, choice groups; I-27 |
| T-E6 `usecase/execute` operations log | ✅ Done | Row before run, consent on every row, records/grants, skip dependents, no retry; `StateDir.OpenLog` (I-28) |
| T-E7 `usecase/execute` re-plan and cancel | ✅ Done | `Replanner` port, `already-present`, `context.WithoutCancel` per command; download failure exit 4 (I-29) |
| T-E8 `usecase/doctor` and `usecase/status` | ✅ Done | Whole-catalog pre-flight, tap trust (I-30); status rows with provenance |
| T-E9 `usecase/summary` | ✅ Done | No ports; caller maps the execute outcome (I-31) |
| T-F1 `adapter/report` schema v1 | ✅ Done | Embedded schema, every command validated in tests (I-33); also `adapter/installer` (I-32), `scan.Input.ReadOnly` (I-34), child process groups (I-35) |
| T-F2 `adapter/cli` command tree and exit codes | 🔄 In Progress | |

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
| 2026-11-14 | Step 3, workstream D: `brew` (locator, shellenv, info parsing, detector, formula/cask backends, tap/trust state), `detect` (PATH, bundle, npm), `fsprobe`; detection-only `gemini-cli` entry (T-D9). Coverage: 95.6% overall. Deviations I-16 to I-24. |
| 2026-11-14 | Step 3, workstream E: bootstrap, scan, conflicts, planning, execute, doctor, status and summary use cases; `inventory.InstallRef` shared by scan and execute; `statedir.OpenLog`. Coverage: 96.4% overall, 100% domain, usecase ≥ 97.2% per package. Deviations I-26 to I-31. |
