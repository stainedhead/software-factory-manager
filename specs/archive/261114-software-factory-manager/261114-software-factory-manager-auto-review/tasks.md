# Tasks: software-factory-manager auto-review fixes

**Date:** 2026-11-14
**Status:** Implementation

## Progress Summary

0 / 26 tasks complete. Live progress is in [status.md](status.md).

Format: ID, FR, dependencies, estimate, acceptance criteria (full text in the PRD).

## Phase 1: P1 fixes

- **T-1a** (FR-001a) — deps: none — 1 h — `catalog.Validate` allow-lists for package ID, brew install ID, tap and detect commands; control characters rejected; embedded catalog passes; e2e `plan --catalog` with `id: "-f"` exits 3 with no brew call.
- **T-1b** (FR-001b) — deps: none — 30 min — `plan.Validate` rejects every brew option after the subcommand except `--cask`/`--formula` on `install`/`trust`.
- **T-1c** (FR-001c) — deps: none — 45 min — `Command.String` escapes control characters; Review golden shows the escaped form.
- **T-2** (FR-002) — deps: none — 1 h — relative PATH entries skipped in `execrunner` and `detect`; `path-relative-entry` warning; e2e with `.` first in PATH.
- **T-3** (FR-003) — deps: none — 1 h — real `brew info` shapes where a trustworthy source exists; field-test checklist for the clean-machine run; deferral rationale.

## Phase 2: P2 fixes

- **T-1d** (FR-001d) — `statedir.LogPath` sanitizes the package ID.
- **T-4** (FR-004) — `make integration -v`; mutate mode fails instead of skipping.
- **T-5a** (FR-005a) — atomic migration under `BEGIN IMMEDIATE`; 20-way concurrent open test.
- **T-5b** (FR-005b) — "PID unknown" for empty or garbage lock files.
- **T-6** (FR-006) — second interrupt escalates SIGTERM then SIGKILL; truthful first message.
- **T-7a** (FR-007a) — operation log footer.
- **T-7b** (FR-007b) — signal-killed exit `128+signo`, `killed by signal <NAME>`.
- **T-8** (FR-008) — probe log and `reason` on unknown detections.
- **T-9** (FR-009) — installer over-limit error, https-only redirects, SHA-256 and size recorded.
- **T-10a** (FR-010a) — move pre-flight/re-plan rules to the use-case layer.
- **T-10b** (FR-010b) — `adapter/tui` no longer imports `adapter/cli`.
- **T-10c** (FR-010c) — AGENTS.md exception documented; `make archcheck`.
- **T-11** (FR-011) — invalid plan never printed.
- **T-12** (FR-012) — located brew run by absolute path.
- **T-13** (FR-013) — inherited `HOMEBREW_*` names shown; `homebrew-cask-opts` warning.
- **T-14** (FR-014) — fuzz targets and `make fuzz`.
- **T-15** (FR-015) — AGENTS.md Commands table complete and checked.
- **T-16** (FR-016) — Paperclip reason once.
- **T-17** (FR-017) — acceptance evidence, field-check guide, format docs.
- **T-18** (FR-018) — status notes on future-increment guides, checked.
- **T-19** (FR-019) — CI `make build-all`; actions pinned or ADR rationale.
