# Status: software-factory-manager auto-review fixes

**Feature:** Increment 1 review fixes (`facman`, the Agentic Software Factory Manager)
**Spec directory:** `specs/261114-software-factory-manager-auto-review/`
**Created:** 2026-11-14
**Orchestration dashboard:** [../archive/261114-software-factory-manager/software-factory-manager-status.md](../archive/261114-software-factory-manager/software-factory-manager-status.md)

> **MANDATORY:** update this file after every task and phase.

## Overall Progress

| Phase | Name | Status | Notes |
|---|---|---|---|
| 0 | Initial research (PRD) | Complete | Review PRD moved into this directory |
| 1 | Specification (spec.md) | Complete | 26 requirements from the review PRD |
| 2 | Research & data modeling | Complete | research.md, data-dictionary.md |
| 3 | Architecture & planning | Complete | architecture.md, plan.md |
| 4 | Task breakdown | Complete | tasks.md: 26 tasks (T-1a to T-19) |
| 5 | Implementation | In Progress | 8 / 26 tasks closed (done or deferred) |
| 6 | Completion & archival | Not Started | |

## Phase 0 Checklist

- [x] Spec directory created (`specs/261114-software-factory-manager-auto-review/`)
- [x] Review PRD moved into the spec directory (link fixed for the new location)
- [x] Research questions identified (research.md RQ-1 to RQ-5)
- [x] Phase files initialized

## Phase 5 Task Log

| Task | FR | Pri | Status | Notes |
|---|---|---|---|---|
| T-1a | FR-001a | P1 | ✅ Done | Allow-lists in `domain/catalog/values.go`; e2e `TestCatalogValuesRejectedAtLoad` (RF-1 to RF-3) |
| T-1b | FR-001b | P1 | ✅ Done | Brew option allow-list and `forceLike` in `plan.Validate`; testkit oracle hardened (RF-4) |
| T-1c | FR-001c | P1 | ✅ Done | `Command.String` escapes non-printables; TUI golden `review-escaped`, headless test (RF-5) |
| T-2 | FR-002 | P1 | ✅ Done | Relative/empty PATH entries skipped in `execrunner`, `detect` and `brew.Locate`; `path-relative-entry` warning; e2e `TestRelativePathEntryNeverRuns` (RF-6, RF-7) |
| T-3 | FR-003 | P1 | 🟡 Partial (rest deferred) | Shape checker + read-only real-brew contract test (CI logs, `FACMAN_CAPTURE_DIR`); `user-docs/field-check.md` clean-machine and capture steps. Container bootstrap job and captured fixtures deferred (RF-8, RF-9) |
| T-1d | FR-001d | P2 | ✅ Done | `statedir.LogPath` sanitizes the package ID; `TestLogPathStaysInLogs` |
| T-4 | FR-004 | P2 | ⬜ Not started | |
| T-5a | FR-005a | P2 | ⬜ Not started | |
| T-5b | FR-005b | P2 | ✅ Done | `inventory.HolderLabel`: "PID unknown" in all three lock messages (RF-10) |
| T-6 | FR-006 | P2 | ⬜ Not started | |
| T-7a | FR-007a | P2 | ⬜ Not started | |
| T-7b | FR-007b | P2 | ⬜ Not started | |
| T-8 | FR-008 | P2 | ⬜ Not started | |
| T-9 | FR-009 | P2 | ✅ Done | 4 MiB limit is an error, https-only redirects, SHA-256 and size in the log and `results[].installer` (RF-11) |
| T-10a | FR-010a | P2 | ⬜ Not started | |
| T-10b | FR-010b | P2 | ⬜ Not started | |
| T-10c | FR-010c | P2 | ⬜ Not started | |
| T-11 | FR-011 | P2 | ⬜ Not started | |
| T-12 | FR-012 | P2 | ⬜ Not started | |
| T-13 | FR-013 | P2 | ⬜ Not started | |
| T-14 | FR-014 | P2 | ⬜ Not started | |
| T-15 | FR-015 | P2 | ⬜ Not started | |
| T-16 | FR-016 | P2 | ⬜ Not started | |
| T-17 | FR-017 | P2 | ⬜ Not started | |
| T-18 | FR-018 | P2 | ⬜ Not started | |
| T-19 | FR-019 | P2 | ⬜ Not started | |

## Blockers

None.

## Open Items

- **FR-003 (J):** clean-machine bootstrap on a macOS VM and real `brew` output capture: [user-docs/field-check.md](../../user-docs/field-check.md) parts 2 and 3; record the run in implementation-notes.md, *macOS clean-machine run*.

## Recent Activity

| Date | Activity |
|---|---|
| 2026-11-14 | Spec created from the review PRD (dev-flow Step 8) |
| 2026-11-14 | FR-001a, FR-001b, FR-001c done (P1) |
