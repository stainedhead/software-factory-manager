# Dev-Flow Process Analysis

**Feature:** software-factory-manager (`facman` Increment 1: Phase 0 + Phase 1 MVP, plus its review fixes)
**Spec directory:** `specs/archive/261114-software-factory-manager/`
**Report generated:** 2026-11-14
**Orchestrator:** `/implm-frm-prd` (dev-flow), run unattended in a Claude Code cloud session on branch `claude/project-thread-ys3kmd`
**Sources:** `git log --format="%h %aI %s"` (authoritative for timing), [software-factory-manager-status.md](software-factory-manager-status.md), [spec.md](spec.md), [status.md](status.md), [tasks.md](tasks.md), the nested [review spec](261114-software-factory-manager-auto-review/)

---

## 1. Executive Summary

Increment 1 delivers the smallest useful slice of `facman`: on a macOS or Linux machine a developer runs `facman` (TUI) or `facman apply --profile agentic-factory` (headless), and `facman` checks the bootstrap state, detects every catalog v1 package and how it got there, classifies each into an ownership state, and proposes only the missing brew-installable tools with the exact commands. After explicit consent it installs them through the `brew-formula` and `brew-cask` backends and records every step in a local SQLite inventory with an append-only audit log, never touching what it did not install, never overwriting a conflicting file, and trusting Orca's tap item by item. A code review then produced 26 fix requirements, of which 21 were done, 3 partly done and 2 deferred with rationale.

**Total runtime:** 4 h 12 min of wall clock by git (4 h 06 min from the dashboard's Process Start to Process End, 2026-11-14T22:44:04Z). The git history runs from the first run commit `3c40123` (2026-11-14T18:31:27Z) to the archive commit (Step 13, see section 3). The PRD review began at about 18:30Z, and the dashboard's Process Start is 18:37:38Z.
**Overall assessment:** Successful. Steps 1 to 13 ran unattended and nothing was escalated. Step 14's PR (#2) has been open since the start of the run. The result is about 28.7 k lines of Go (14.0 k of them tests, 297 top-level tests), 96.5% coverage (domain 100%, use cases 97.4 to 100%), and CI green on Linux and macOS. I1-1 to I1-15 are proven by tests. I1-16, the clean-machine bootstrap and the real `brew` output capture remain for J's field run ([user-docs/field-check.md](../../../user-docs/field-check.md)). The biggest risk is that the real-machine checks have not happened yet, not the code.

---

## 2. Step-by-Step Timing

Start and end times are from the status dashboard. They agree with the git commit that closes each step to within about a minute, and git is used where they differ.

| Step | Name | Start | End | Runtime (min) | Key Outputs |
|---|---|---|---|---|---|
| pre | PRD review (`/dev-flow:review-prd`) and run setup | ~18:30Z | 18:36:54Z (`eb66dbb`) | ~7 | Plugin enabled (`3c40123`), CI tidy fix (`08d0abb`), PRD Appendix L, Increment 1 scope |
| 1 | Create Spec from PRD | 18:40:04Z | 18:49:31Z | 9 | `specs/261114-software-factory-manager/`, 52 tasks in 8 workstreams, I1-1 to I1-16 |
| 2 | Review Spec | 18:49:31Z | 18:51:30Z | 2 | Needs revision → implementation-ready (R-1 to R-9) |
| 3 | Implement Product | 18:53:07Z | 21:10:52Z | 137 | Workstreams A to H, 11 commits, coverage 96.6% |
| 4 | Documentation and User Docs | 21:10:52Z | 21:19:50Z | 9 | README, INTENT, docs, design v0.4, user-docs; Ctrl+C fix (`11fed13`, git 21:20:01Z) |
| 5 | Code and Design Review | 21:23:41Z | 21:32:42Z | 9 | Review PRD: 19 findings (0 P0, 3 P1, 16 P2) |
| 6 | Prepare Review PRD | 21:33:34Z | 21:34:52Z | 1 | 26 atomic requirements |
| 7 | Archive Original Spec | 21:35:22Z | 21:36:21Z | 1 | Spec moved to `specs/archive/`, 0 broken links |
| 8 | Spec Review Fixes | 21:37:53Z | 21:39:38Z | 2 | Review spec, T-1a to T-19 |
| 9 | Implement Review Fixes | 21:39:38Z | 22:34:10Z | 55 | 9 commits; archcheck, doccheck and fuzz gates; Homebrew 7 cask parser fix |
| 10 | Archive Fixes Spec | 22:34:29Z | 22:35:44Z | 1 | Review spec nested in the Increment 1 archive |
| 11 | Final Quality Pass | 22:37:04Z | 22:41:14Z | 4 | All gates green; three doc fixes (`d9b289f`) |
| 12 | Process Analysis Report | 22:41:20Z | 22:43:43Z | 2 | This file (`343b75a`) |
| 13 | Archive Spec | 22:43:51Z | 22:44:04Z | 1 | Layout confirmed; Process End and Total Runtime set |
| 14 | Open Pull Request | — | — | — | PR #2 was opened at 18:32:09Z, at the start of the run (see section 3) |

**Notable observations:**
- Step 3 took 54% of the run (137 of 252 minutes), and Step 9 took another 22%. Everything else was one to nine minutes per step.
- There was no idle gap between steps longer than 4 minutes (Step 4 → 5: 21:19:50 to 21:23:41, waiting for macOS CI on the Ctrl+C fix).
- No step was retried as a whole. Three fixes were driven by CI (section 6). Each was a focused follow-up commit, not a re-run.

---

## 3. Commit and Push Summary

**Total commits in the run:** 35 from `3c40123` to `d9b289f`, plus this report and the Step 13 archive commit. The three commits authored at 18:02 to 18:22Z (`e74092e`, `6e7196d`, `f0776c9`) are J's scaffolding on `main` before the run and are not counted. Every commit was pushed to `claude/project-thread-ys3kmd` right after it was made.

| Commit | Timestamp (UTC) | Message |
|---|---|---|
| 3c40123 | 18:31:27 | Enable the dev-flow plugin for this repo |
| 08d0abb | 18:32:52 | Fix CI tidy check when go.sum does not exist |
| eb66dbb | 18:36:54 | Review PRD and plan Increment 1 |
| 50e4528 | 18:49:43 | Create Increment 1 spec from the PRD |
| 14ff48b | 18:51:37 | Review the Increment 1 spec and resolve its gaps |
| 361ce3b | 19:11:26 | Add Increment 1 domain model, ports and test harness |
| 8867172 | 19:16:41 | Add embedded catalog v1, profiles and YAML loader |
| 2ef6621 | 19:25:02 | Add runner, platform, state dir and SQLite inventory adapters |
| 7a8a72d | 19:42:39 | Add detectors, Homebrew backends and conflict probe |
| 092c7a9 | 19:46:56 | Resolve symlinked locations when classifying paths |
| 7c623ea | 19:52:27 | Add bootstrap, scan, conflicts and planning use cases |
| 5064c5d | 20:04:33 | Add execute, doctor, status and summary use cases |
| ff18189 | 20:16:20 | Add report schema v1 and the Homebrew installer adapter |
| 45298bd | 20:36:53 | Add the headless CLI, session, wiring and end-to-end tests |
| 743cd13 | 20:59:00 | Add selection, plan-command runner and hand-off seams for the TUI |
| 1fa3b28 | 20:59:01 | Add the facman TUI install flow |
| cf638ce | 21:11:37 | Add real-Homebrew integration tests, CI wiring and Increment 1 docs |
| 075beb9 | 21:13:03 | Document Increment 1 for adopters (dev-flow Step 4) |
| 11fed13 | 21:20:01 | Treat Ctrl+C before consent as a decline in apply |
| 54aa3d6 | 21:33:11 | Add the Increment 1 auto-review PRD (dev-flow Step 5) |
| c37abc9 | 21:35:12 | Review and finalize the auto-review PRD (dev-flow Step 6) |
| 1815d07 | 21:36:49 | Archive the Increment 1 spec (dev-flow Step 7) |
| 64a34bc | 21:40:10 | Create the auto-review fixes spec (dev-flow Step 8) |
| c284730 | 21:47:59 | Validate catalog values and forbid every brew option from data |
| c89e84b | 21:52:12 | Never resolve programs from relative PATH entries |
| 2006a3a | 21:55:16 | Add a real-Homebrew contract test and the field-check procedure |
| ca79e90 | 21:59:36 | Harden log names, lock messages and the installer download |
| 44eb85b | 22:04:00 | Read Homebrew 7 cask link targets in brew info |
| 847bc59 | 22:10:32 | Record command outcomes and never print an unsafe plan |
| adb75ad | 22:14:00 | Make I1-14 runs auditable and the schema migration atomic |
| 6e1022d | 22:21:52 | Run the located brew and name inherited HOMEBREW_* settings |
| 6c18c58 | 22:29:09 | Enforce layer rules, fuzz parsers and check the docs in make check |
| 215bb24 | 22:34:08 | Close the acceptance record and say truthfully what Ctrl+C does |
| 1e12276 | 22:36:11 | Archive the review-fixes spec inside the Increment 1 archive |
| d9b289f | 22:41:14 | Finish the final quality pass: Increment 1 is stable |

**Pull requests:** PR #1 (INTENT.md, design and the `facman` name) was merged by J before the run, at 17:57:21Z. PR #2, *feat: facman Increment 1 (MVP) via dev-flow implm-frm-prd*, was opened at 18:32:09Z through the GitHub connector, not `gh pr create`. It was opened early so J could follow CI from a phone. It stays open for J to merge.

**CI:** Linux and macOS ran on every push. The run for `215bb24` was cancelled because the next push, `1e12276`, superseded it in the same concurrency group. Every other completed run since the macOS fixes landed is green.

---

## 4. Spec vs. Implementation Comparison

The tasks in tasks.md are estimated in focused engineering hours (53 tasks, 132.5 h). The review-fix spec estimated only its three P1 tasks (1 h each).

| Phase | Planned (spec) | Actual (git log) | Difference | Notes |
|---|---|---|---|---|
| PRD review and spec (Steps pre, 1, 2) | N/A | ~21 min (18:30 → 18:51:37) | — | `prd-to-spec` not installed; inline templates used |
| Research and architecture | Folded into spec authoring | Within Step 1 | — | research, data dictionary, architecture and plan written in one pass |
| Implementation, workstreams A to H | 132.5 h (A 24, B 10, C 13, D 21, E 25, F 19, G 12.5, H 8) | 137 min (18:53 → 21:10:52) | about 58× faster than the estimate | Sequential workers, not parallel worktrees |
| Docs (H and Step 4) | 8 h, part of H | 9 min plus the docs in `cf638ce` | — | |
| Review and fixes (Steps 5 to 10) | 3 h estimated for the P1s; the P2s not estimated | 72 min (21:23:41 → 22:35:44) | — | 21 of 26 done, 3 partial, 2 deferred |
| Testing | Inside each task (TDD) | Inside each commit | — | 14.0 k lines of tests; e2e on a pty; fuzz added in Step 9 |

**Phases skipped:** none. Steps the commands describe as interactive (confirmation prompts, the optional PRD validation, `/archive-spec` confirmations) were decided by the orchestrator and logged in the dashboard's run notes.
**Phases added:** the T-D9 task (the symlink-resolution fix found by macOS CI), the Step 4 Ctrl+C fix (I-54), the FR-005 short-flag normalizer shipped early (I-36, recorded deviation), and three new gates (`archcheck`, `doccheck`, `fuzz`) that came out of the review.

---

## 5. Token / Message Usage

Exact token counts are not available in this session, and turn counts were not recorded, so no numbers are reported. Only the shape is known:
- **Orchestrator:** one session that ran the pre-step and Steps 1 to 14 bookkeeping, CI polling, decisions and dashboard updates.
- **Worker agents:** five sequential implementation workers in Step 3 (A–C, D–E, F, G, then H with docs), one review-fix worker in Step 9 (RF-0), and one worker for Steps 11 to 13. Steps 3 and 9 were by far the largest consumers. Steps 1, 2, 6, 7, 8 and 10 were short.
- All workers used the session's current model. The command's pinned `claude-sonnet-4-6` could not be selected from this session.

---

## 6. Process Observations

### What worked well
- **The PRD's Delivery plan scoped the run.** Cutting the PRD to Increment 1 in Step 1, with every other requirement listed as deferred, kept a large PRD to a buildable slice and gave the review a clear scope.
- **The acceptance criteria were testable.** Each of I1-1 to I1-15 maps to named tests ([technical-details.md, Acceptance evidence](../../../docs/technical-details.md#acceptance-evidence)). The shared invocation-log safety check at the end of every e2e test enforces I1-8 and I1-10 across the whole suite rather than in one place.
- **macOS CI from the start.** The cloud container is Linux, runs as root and has no Homebrew. Running CI on macOS on every push was the only real-platform signal, and it caught three bugs the container could not see:
  1. **The `/var` symlink** (`7a8a72d` → `092c7a9`): macOS TMPDIR is under `/var → /private/var`, so resolved paths never matched the unresolved prefixes, and every probe came out `orphan`.
  2. **Ctrl+C at the consent prompt** (`11fed13`): a racy test hid a real bug, where Ctrl+C before consent during the scan exited 4 instead of counting as a decline.
  3. **The Homebrew 7.0.11 cask artifact shape** (`44eb85b`): the new real-`brew` contract test found that cask link `target` entries changed shape, and the parser was fixed.
- **The review loop paid off.** The Step 5 review (3 P1s: unvalidated catalog values flowing into argv, paths and the terminal; relative PATH entries; no real bootstrap run) found real safety gaps that the tests did not, and Step 9 closed them TDD-first.
- **Opening the PR early** gave J a place to watch CI and progress from a phone while away.

### What caused delays or rework
- **The plugin could not be installed.** The dev-flow plugin failed install validation because the `commands` paths in its `plugin.json` lack a `./` prefix. The orchestrator read and followed the command files in `shared-plugins/plugins/dev-flow/commands/` directly. That worked, but it is a manual step, and nested slash commands (`/create-spec`, `/review-code`, `/write-flow-analys`) were followed as documents, not invoked.
- **Sequential workers, not parallel worktrees.** The plan allowed parallel workstreams, but A–C (domain, ports, test harness) underpin everything, and the later workstreams share `go.mod`, the wiring in `cmd/facman` and the e2e harness. To avoid merge conflicts in an unattended run, the workers ran one after another (A–C, D–E, F, G, then H with docs). That cost wall-clock time in Step 3, though probably less than resolving cross-worktree conflicts with nobody to ask.
- **Workers did not use the pinned model.** The command pins `claude-sonnet-4-6` for worker agents, but this session could only start workers on its own model. Quality was not affected, but runs are not directly comparable with runs that use the pinned model.
- **CI round trips.** Each macOS-only bug cost one push-and-wait cycle (5 to 10 minutes). The Step 4 → 5 gap was spent waiting on one.
- **Deferred and partial review items.** FR-008 (probe log) waits for the I1-16 field data, and FR-010a (moving the session rules into the use-case layer) waits for Increment 2. FR-003, FR-006 and FR-019 are partly done. Each has a rationale in the review spec's implementation-notes.md, but they are open work carried into the next increment.
- **`gh` limits in the cloud.** GraphQL is blocked from the session, so `gh pr create` (Step 14 as written) does not work there, and `gh pr list` fails too. The PR went through the GitHub connector, and CI was read through `gh api` REST calls.

### Recommendations for future runs
For the dev-flow plugin:
1. **Fix `plugin.json`.** Prefix every `commands` path with `./` so the plugin passes install validation and the nested slash commands can be invoked properly.
2. **Make the worker model configurable.** Treat `claude-sonnet-4-6` as a default, not a requirement, and have the orchestrator record which model actually ran.
3. **Document a sequential fallback for implementation.** Let `implm-frm-prd` choose sequential workers when the workstreams share a foundation or the run is unattended, and say how to split the work (foundation first, then leaves).
4. **Support hosts without `gh`.** In Step 14, allow the GitHub connector or `gh api` REST as alternatives to `gh pr create`, and allow opening a draft PR at the start for remote monitoring, then updating its body in Step 14.
5. **Add an explicit unattended mode.** It would skip confirmation prompts, choose the safest default, and log each decision to the dashboard, which this run did by hand.
6. **Fix the archive-spec chicken-and-egg.** `/archive-spec` requires 100% in status.md, but archiving is Phase 6. Let Phase 6 be marked complete as part of the archive step.
7. **Make CI part of the gate.** After each implementation push, wait for CI on every target OS before the step is marked complete, especially when the build host differs from the target platform.
8. **Ship the `prd-to-spec` skill with the plugin**, or make the inline templates in `create-spec.md` the documented default.

For this project:
- J should run [user-docs/field-check.md](../../../user-docs/field-check.md) (I1-16 on 3 to 5 lab Macs, the clean-machine bootstrap on a macOS VM, and `brew` output capture) before Increment 2 builds on the detection rules.
- Start Increment 2 with the deferred items FR-010a and FR-006 AC1 (with an ADR).

---

## 7. Manual vs. Automated Comparison

**Estimated manual duration:** about 4 to 5 weeks for one senior Go developer. The basis is the spec's own 132.5 focused hours for Increment 1, at about 30 focused hours a week (about 4.5 weeks), plus about 3 to 5 days for the PRD and spec review, the code review and the 26 review fixes. This assumes the developer already knows Homebrew's internals and Bubble Tea, and it excludes meetings and review wait time.
**Actual automated runtime:** 4 h 12 min of wall clock (git, `3c40123` 18:31:27Z to the Step 13 commit), with no human input during the run.
**Efficiency gain:** roughly 30 to 40 times in elapsed working time, with large caveats. J's review of PR #2, the field check and the clean-machine bootstrap have not happened yet. They need human time and real Macs, and they could find rework. The automated run also leaned on a well-structured PRD and design (v0.3 and v0.4) that took human effort to produce before the run began.
