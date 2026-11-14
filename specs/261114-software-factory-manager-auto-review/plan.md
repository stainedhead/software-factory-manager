# Plan: software-factory-manager auto-review fixes

**Date:** 2026-11-14
**Status:** Planning → Implementation

## Development Approach

TDD for every fix: a failing test reproducing the finding, the minimal change, then refactor. After each fix, a short self-review of the diff (correctness, Clean Architecture direction, and that no new path can pass `--force`, `-f` or `--overwrite` or grant whole-tap trust), recorded in [implementation-notes.md](implementation-notes.md).

Run as a single sequential worker. The PRD suggests teammates and worktrees per stream; this run is one unattended worker, so streams run one after the other in priority order, which also removes the A-before-D merge-order risk.

## Phase Breakdown

1. **P1 (stream A + B):** T-1a, T-1b, T-1c, T-2, then T-3 (partial and field checklist).
2. **P2 quick hardening:** T-1d, T-5b, T-9, T-11, T-16, T-7b, T-7a, T-12, T-13.
3. **P2 tooling and docs:** T-4, T-10b, T-10c, T-15, T-18, T-17, T-19, T-14.
4. **P2 larger items:** T-5a, T-6, T-8, T-10a, each done or deferred with a rationale.

## Critical Path

T-1a → T-1b → T-1c (the consent-screen safety chain). Everything else is independent.

## Testing Strategy

Unit tests beside the code, table-driven, fakes over mocks; e2e tests in `cmd/facman` through fakecmd. No real Homebrew, installs or `sudo` anywhere.

## Rollout Strategy

Commit and push each stable group to `claude/project-thread-ys3kmd`. No release.

## Success Metrics

`make check` green; `make cover` ≥ 80% overall, ≥ 90% domain and usecase; the FR-001 reproduction exits 3.
