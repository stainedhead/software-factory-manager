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
| 5 | Implementation | In Progress | 26 / 26 tasks closed (done or deferred) |
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
| T-3 | FR-003 | P1 | 🟡 Partial (rest deferred) | Shape checker + read-only real-brew contract test; it found the Homebrew 7.0.11 cask `target` shape on CI, parser fixed (RF-12); field-check procedure. Container bootstrap job deferred (RF-8, RF-9) |
| T-1d | FR-001d | P2 | ✅ Done | `statedir.LogPath` sanitizes the package ID; `TestLogPathStaysInLogs` |
| T-4 | FR-004 | P2 | ✅ Done | verbose make integration; MUTATE=1 turns skips into failures |
| T-5a | FR-005a | P2 | ✅ Done | BEGIN IMMEDIATE per step; 20-way test passes -count=100 |
| T-5b | FR-005b | P2 | ✅ Done | `inventory.HolderLabel`: "PID unknown" in all three lock messages (RF-10) |
| T-6 | FR-006 | P2 | 🟡 Partial (rest deferred) | AC2 truthful interrupt text done; AC1 second-interrupt kill deferred (consent model, needs ADR) |
| T-7a | FR-007a | P2 | ✅ Done | # result/# time footer in Run and Handoff |
| T-7b | FR-007b | P2 | ✅ Done | 128+signo and killed by signal NAME; e2e |
| T-8 | FR-008 | P2 | ⏸️ Deferred | probe log + unknown reasons; with I1-16 feedback (rationale in notes) |
| T-9 | FR-009 | P2 | ✅ Done | 4 MiB limit is an error, https-only redirects, SHA-256 and size in the log and `results[].installer` (RF-11) |
| T-10a | FR-010a | P2 | ⏸️ Deferred | session refactor; with Increment 2 npm backend (rationale in notes) |
| T-10b | FR-010b | P2 | ✅ Done | tui.Request + cli.TUIFunc wired in cmd; no import either way |
| T-10c | FR-010c | P2 | ✅ Done | AGENTS.md exception documented; make archcheck in check and CI |
| T-11 | FR-011 | P2 | ✅ Done | planning and cli never print an invalid plan |
| T-12 | FR-012 | P2 | ✅ Done | located brew runs by absolute path; Review names it |
| T-13 | FR-013 | P2 | ✅ Done | inherited HOMEBREW_* names in Review, header, JSON, logs; homebrew-cask-opts warning |
| T-14 | FR-014 | P2 | ✅ Done | 6 fuzz targets; make fuzz; CI Linux; no crashers |
| T-15 | FR-015 | P2 | ✅ Done | Commands table complete; make doccheck |
| T-16 | FR-016 | P2 | ✅ Done | Paperclip hint removed; e2e TestPaperclipReasonOnce |
| T-17 | FR-017 | P2 | ✅ Done | acceptance evidence table, archived boxes ticked, docs/catalog-format.md |
| T-18 | FR-018 | P2 | ✅ Done | status notes on 6 guides; doccheck asserts them |
| T-19 | FR-019 | P2 | 🟡 Partial (rest deferred) | build-all in CI; tags kept with ADR rationale, SHA pin is follow-up |

## Blockers

None.

## Open Items

- **FR-003 (J):** clean-machine bootstrap on a macOS VM and real `brew` output capture: [user-docs/field-check.md](../../user-docs/field-check.md) parts 2 and 3; record the run in implementation-notes.md, *macOS clean-machine run*.
- **FR-006 AC1 (deferred):** second Ctrl+C stops a hung command; needs a design note and an ADR (it changes the "never killed" promise).
- **FR-008 (deferred):** per-scan probe log and a reason on every `unknown` detection; do it with the I1-16 feedback.
- **FR-010a (deferred):** move pre-flight and re-plan rules from `adapter/session` into a use case, with Increment 2's npm backend.
- **FR-019 (follow-up):** pin the four workflow actions to full commit SHAs (tag in a comment) and add Dependabot for `github-actions`, before the first release tag; see the ADR entry of 2026-11-14.

## Recent Activity

| Date | Activity |
|---|---|
| 2026-11-14 | Spec created from the review PRD (dev-flow Step 8) |
| 2026-11-14 | FR-001a, FR-001b, FR-001c done (P1) |
