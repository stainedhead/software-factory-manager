# Implementation notes: software-factory-manager auto-review fixes

**Date:** 2026-11-14

Purpose: decisions, per-fix reviews, edge cases and deferrals made while implementing the review fixes. Update this file with every non-obvious decision and after each fix's self-review.

## Technical Decisions

- **RF-0 (process).** One sequential worker, no teammates or worktrees: the run is unattended and single-agent, and sequential order removes the PRD's merge-order risk between streams A and D.

## Per-fix reviews

## Edge Cases & Solutions

## Deviations from Plan

## Deferred items

## Lessons Learned
